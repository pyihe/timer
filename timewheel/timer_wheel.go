package timewheel

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyihe/timer"
	"github.com/pyihe/timer/pkg/cronexpr"
	"github.com/pyihe/timer/pkg/gopool"
	"github.com/pyihe/timer/pkg/taskpool"
)

const (
	defaultCursor = -1
	statusOpen    = 0
	statusClosed  = 1
)

type wheelTimer struct {
	slots      []*list.List     // 槽位，每个刻度对应的任务列表
	tasks      *sync.Map        // 任务map，用于标记删除
	closeChan  chan struct{}    // 关闭信号量
	bufferChan chan interface{} // 任务通道，用于新增延时任务
	timeMS     time.Duration    // 精度，每移动一个格子代表走动的时间
	closed     int32            // 是否关闭，timer是否被关闭
	slotCursor int              // 当前时间指针，当前时间指针，随着tick移动
	slotNum    int              // 槽位数量，一共多少个格子，走一圈的时间范围为slotNum * slots
}

func New(timeMs time.Duration, slot, bufferSize int) timer.Timer {
	var w = &wheelTimer{
		timeMS:     timeMs,
		slots:      make([]*list.List, slot),
		slotNum:    slot,
		slotCursor: defaultCursor,
		closed:     statusOpen,
		tasks:      &sync.Map{},
		closeChan:  make(chan struct{}),
	}
	w.bufferChan = make(chan interface{}, bufferSize)

	w.init()
	w.start()
	return w
}

func (w *wheelTimer) init() {
	for i := range w.slots {
		w.slots[i] = list.New()
	}
}

func (w *wheelTimer) start() {
	gopool.Execute(func() {
		ticker := time.NewTicker(w.timeMS)
		for {
			select {
			case <-ticker.C:
				w.onTick()
			case t := <-w.bufferChan:
				switch t.(type) {
				case *taskpool.Task:
					w.addTask(t.(*taskpool.Task))
				case int64:
					w.deleteTask(t.(int64))
				}
			case <-w.closeChan:
				ticker.Stop()
				n := 0
				w.tasks.Range(func(key, value any) bool {
					n += 1
					return true
				})
				gopool.Release()
				return
			}
		}
	})
}

func (w *wheelTimer) isClosed() bool {
	return atomic.LoadInt32(&w.closed) == statusClosed
}

// 获取延时对应的槽位已经需要走动的圈数
func (w *wheelTimer) getPosition(delay time.Duration) (pos, loop int) {
	step := int(delay / w.timeMS)           // step表示该延时需要走动多少个槽位
	pos = (w.slotCursor + step) % w.slotNum // 从当前时间指针对应的槽位开始计算，该延时所属的槽位索引
	loop = (step - 1) / w.slotNum           //
	return
}

func (w *wheelTimer) addTask(t *taskpool.Task) {
	// 对于周期性任务，可能存在这种场景：
	// 任务刚执行完的同时收到了删除该任务的请求
	// 这时做了标记删除
	// 与此同时任务执行完又执行了add流程，导致需要再执行一次才能删除
	// 所以在添加的时候先做删除判断
	if deleted, ok := t.Extra[1].(bool); ok && deleted {
		return
	}
	// 延时不能小于最小精度
	if t.Delay < w.timeMS {
		t.Delay = w.timeMS
	}

	// 计算槽位
	pos := -1
	pos, t.Extra[0] = w.getPosition(t.Delay)

	_, exist := w.tasks.Load(t.ID)
	// 如果已经存在，证明是需要重复执行的任务，不做处理
	if !exist {
		// 存入task map
		w.tasks.Store(t.ID, t)
	}

	// 放入槽位
	w.slots[pos].PushBack(t)
}

// 删除任务，先在task map中标记删除，在onTick遍历任务队列时进行真正的删除
func (w *wheelTimer) deleteTask(taskID int64) {
	v, exist := w.tasks.Load(taskID)
	if !exist {
		return
	}

	v.(*taskpool.Task).Extra[1] = true
	w.tasks.Delete(taskID)
}

// 时间指针（索引）每跳动一次需要执行的流程
func (w *wheelTimer) onTick() {
	// 指针先跳动一下
	w.slotCursor = (w.slotCursor + 1) % w.slotNum

	// 去除跳动后指针对应槽位的任务列表
	execSlot := w.slots[w.slotCursor]
	if execSlot.Len() == 0 {
		return
	}

	// 遍历该列表，收集需要执行的任务
	jobs := make([]func(), 0, execSlot.Len())
	for p := execSlot.Front(); p != nil; {
		t := p.Value.(*taskpool.Task)
		next := p.Next()
		// 如果任务已经被删除，则不需要执行，并将其从任务队列中删除
		if deleted, ok := t.Extra[1].(bool); ok && deleted {
			execSlot.Remove(p)
			p = next
			continue
		}
		// 如果需要走动多层，层数降低
		if loop, ok := t.Extra[0].(int); ok && loop > 0 {
			loop -= 1
			t.Extra[0] = loop
			p = next
			continue
		}
		// 收集可以执行的任务
		jobs = append(jobs, t.Job)

		// 将任务从当前队列删除
		execSlot.Remove(p)
		p = next

		// 判断是否需要重复执行
		switch {
		case t.Repeated: // 如果需要重复执行，重新添加
			gopool.Execute(func() {
				w.bufferChan <- t
			})

		case t.Expr != nil:
			now := time.Now()
			nextTime := t.Expr.Next(now)
			if nextTime.IsZero() {
				w.tasks.Delete(t.ID)
				taskpool.Put(t)
			} else {
				t.Delay = nextTime.Sub(now)
				gopool.Execute(func() {
					w.bufferChan <- t
				})
			}

		default:
			w.tasks.Delete(t.ID) // 否则直接删除
			taskpool.Put(t)      // 回收Task
		}
	}

	w.execTask(jobs...)
}

func (w *wheelTimer) execTask(tasks ...func()) {
	for _, fn := range tasks {
		gopool.Execute(fn)
	}
}

func (w *wheelTimer) After(delay time.Duration, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if w.isClosed() {
		return timer.EmptyTaskID, timer.ErrTimerClosed
	}

	t := taskpool.Get(delay, fn, false, nil)
	gopool.Execute(func() {
		w.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}

func (w *wheelTimer) Every(delay time.Duration, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if w.isClosed() {
		return timer.EmptyTaskID, timer.ErrTimerClosed
	}

	t := taskpool.Get(delay, fn, true, nil)
	gopool.Execute(func() {
		w.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}

func (w *wheelTimer) Delete(id timer.TaskID) error {
	if id == timer.EmptyTaskID {
		return nil
	}
	if w.isClosed() {
		return timer.ErrTimerClosed
	}
	gopool.Execute(func() {
		w.bufferChan <- id
	})
	return nil
}

func (w *wheelTimer) Stop() {
	if !atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return
	}
	close(w.closeChan)
}

func (w *wheelTimer) Cron(desc string, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if w.isClosed() {
		return timer.EmptyTaskID, timer.ErrTimerClosed
	}

	// 解析desc
	expr, err := cronexpr.NewCronExpr(desc)
	if err != nil {
		return timer.EmptyTaskID, err
	}

	var now = time.Now()
	var nextTime = expr.Next(now)
	if nextTime.IsZero() {
		err = timer.ErrInvalidExpr
		return timer.EmptyTaskID, err
	}
	var t = taskpool.Get(nextTime.Sub(now), fn, false, expr)
	gopool.Execute(func() {
		w.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}
