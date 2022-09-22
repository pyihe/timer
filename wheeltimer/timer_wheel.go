package wheeltimer

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/pyihe/go-pkg/snowflakes"
	"github.com/pyihe/timer"
)

type task struct {
	id       int64         // 任务ID
	fn       func()        // 任务
	delay    time.Duration // 延时时间
	repeated bool          // 是否重复执行
	deleted  bool          // 是否删除
	loop     int           // 到达任务执行时间需要走多少圈
}

func (t *task) reset() {
	*t = task{}
}

type wheelTimer struct {
	tasks       *sync.Map         // 任务map，用于标记删除
	taskPool    *sync.Pool        // 任务池，避免频繁分配内存
	idGenerator snowflakes.Worker // ID生成器，用于生成新增任务的唯一ID
	timeMS      time.Duration     // 精度，每移动一个格子代表走动的时间
	slots       []*list.List      // 槽位，每个可读对应的任务列表
	closed      int32             // 是否关闭，timer是否被关闭
	slotNum     int               // 槽位数量，一共多少个格子，走一圈的时间范围为slotNum * slots
	slotCursor  int               // 当前时间指针，当前时间指针，随着tick移动
	closeChan   chan struct{}     // 关闭信号量
	bufferChan  chan interface{}  // 任务通道，用于新增延时任务
}

func New(timeMs time.Duration, slot int, opts ...timer.Option) timer.Timer {
	var c = &timer.Options{
		Node:         0,    // 默认节点ID为0
		GoPoolSize:   1000, // 默认协程池大小为64
		TaskChanSize: 1024, // 默认通道缓冲区大小为1024
	}
	var w = &wheelTimer{
		timeMS:     timeMs,
		slots:      make([]*list.List, slot),
		slotNum:    slot,
		slotCursor: -1,
		closed:     0,
		tasks:      &sync.Map{},
		taskPool: &sync.Pool{
			New: func() interface{} {
				return &task{}
			}},
		closeChan: make(chan struct{}),
	}

	for _, op := range opts {
		op(c)
	}
	w.bufferChan = make(chan interface{}, c.TaskChanSize)
	w.idGenerator = snowflakes.NewWorker(c.Node)

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
	ants.Submit(func() {
		ticker := time.NewTicker(w.timeMS)
		for {
			select {
			case <-ticker.C:
				w.onTick()
			case t := <-w.bufferChan:
				switch t.(type) {
				case *task:
					w.addTask(t.(*task))
				case int64:
					w.deleteTask(t.(int64))
				}
			case <-w.closeChan:
				ticker.Stop()
				ants.Release()
				return
			}
		}
	})
}

func (w *wheelTimer) getTask() *task {
	t, ok := w.taskPool.Get().(*task)
	if ok {
		return t
	}
	t = &task{}
	return t
}

func (w *wheelTimer) putTask(t *task) {
	if t == nil {
		return
	}
	t.reset()
	w.taskPool.Put(t)
}

func (w *wheelTimer) isClosed() bool {
	return atomic.LoadInt32(&w.closed) == 1
}

// 获取延时对应的槽位已经需要走动的圈数
func (w *wheelTimer) getPosition(delay time.Duration) (pos, loop int) {
	step := int(delay / w.timeMS)           // step表示该延时需要走动多少个槽位
	pos = (w.slotCursor + step) % w.slotNum // 从当前时间指针对应的槽位开始计算，该延时所属的槽位索引
	loop = (step - 1) / w.slotNum           //
	return
}

func (w *wheelTimer) addTask(t *task) {
	// 延时不能小于最小精度
	if t.delay < w.timeMS {
		t.delay = w.timeMS
	}

	// 计算槽位
	pos := -1
	pos, t.loop = w.getPosition(t.delay)

	_, exist := w.tasks.Load(t.id)
	// 如果已经存在，证明是需要重复执行的任务，不做处理
	if !exist {
		// 存入task map
		w.tasks.Store(t.id, t)
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
	t := v.(*task)
	t.deleted = true
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
	tasks := make([]func(), 0, execSlot.Len())
	for p := execSlot.Front(); p != nil; {
		t := p.Value.(*task)
		next := p.Next()
		// 如果任务已经被删除，则不需要执行，并将其从任务队列中删除
		if t.deleted {
			execSlot.Remove(p)
			p = next
			continue
		}
		// 如果需要走动多层，层数降低
		if t.loop > 0 {
			t.loop -= 1
			p = next
			continue
		}
		// 收集可以执行的任务
		tasks = append(tasks, t.fn)

		// 将任务从当前队列删除
		execSlot.Remove(p)
		p = next

		// 判断是否需要重复执行
		switch {
		case t.repeated: // 如果需要重复执行，重新添加
			w.addTask(t)
		default:
			w.tasks.Delete(t.id) // 否则直接删除
			w.putTask(t)
		}
	}

	w.execTask(tasks...)
}

func (w *wheelTimer) execTask(tasks ...func()) {
	for _, fn := range tasks {
		fn := fn
		ants.Submit(func() {
			fn()
		})
	}
}

func (w *wheelTimer) After(delay time.Duration, fn func()) (int64, error) {
	if w.isClosed() {
		return 0, timer.ErrTimerClosed
	}
	t := w.getTask()
	t.delay = delay
	t.fn = fn
	t.id = w.idGenerator.GetInt64()
	t.repeated = false
	t.deleted = false

	err := ants.Submit(func() {
		w.bufferChan <- t
	})
	return t.id, err
}

func (w *wheelTimer) Every(delay time.Duration, fn func()) (int64, error) {
	if w.isClosed() {
		return 0, timer.ErrTimerClosed
	}
	t := w.getTask()
	t.delay = delay
	t.fn = fn
	t.id = w.idGenerator.GetInt64()
	t.repeated = true
	t.deleted = false

	err := ants.Submit(func() {
		w.bufferChan <- t
	})
	return t.id, err
}

func (w *wheelTimer) Delete(id int64) error {
	if w.isClosed() {
		return timer.ErrTimerClosed
	}
	return ants.Submit(func() {
		w.bufferChan <- id
	})
}

func (w *wheelTimer) Stop() {
	if !atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return
	}
	close(w.closeChan)
}
