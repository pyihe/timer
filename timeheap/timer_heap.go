package timeheap

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyihe/timer"
	"github.com/pyihe/timer/pkg/cronexpr"
	"github.com/pyihe/timer/pkg/gopool"
	"github.com/pyihe/timer/pkg/taskpool"
)

const bucketLen = 64

type asynHandler func(func())

type heapTimer struct {
	timeBuckets [bucketLen]*timeBucket // bucket
	recycleChan chan *taskpool.Task    // 用于回收task变量的通道
	cancel      context.CancelFunc     // 停止所有协程（不包括执行任务的协程）
	bufferChan  chan interface{}       // 用于添加、删除任务的通道
	taskMap     *sync.Map              // key: task.id	value: bucket index
	closed      int32                  // timer是否关闭
	bucketPos   int                    // bucket索引
}

func New(bufferSize int) timer.Timer {
	var ctx context.Context
	var h = &heapTimer{
		taskMap:     &sync.Map{},
		timeBuckets: [64]*timeBucket{},
		recycleChan: make(chan *taskpool.Task, bucketLen),
		closed:      0,
		bucketPos:   -1,
		bufferChan:  make(chan interface{}, bufferSize),
	}
	ctx, h.cancel = context.WithCancel(context.Background())

	h.init()
	h.start(ctx)
	return h
}

func (h *heapTimer) init() {
	for i := range h.timeBuckets {
		h.timeBuckets[i] = newTimeBucket(h.recycleChan, h.exec)
	}
}

func (h *heapTimer) start(ctx context.Context) {
	// 开启每个桶的任务监控协程
	for _, tb := range h.timeBuckets {
		tb := tb
		gopool.Execute(func() {
			tb.start(ctx)
		})
	}

	gopool.Execute(func() {
		h.run(ctx)
	})

	gopool.Execute(func() {
		h.recycle(ctx)
	})
}

func (h *heapTimer) recycle(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case t := <-h.recycleChan:
			if t != nil {
				h.taskMap.Delete(t.ID)
				taskpool.Put(t)
			}
		}
	}
}

func (h *heapTimer) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-h.bufferChan:
			switch t.(type) {
			case *taskpool.Task:
				h.addTask(t.(*taskpool.Task))
			case int64:
				h.deleteTask(t.(int64))
			}
		}
	}
}

func (h *heapTimer) isClosed() bool {
	return atomic.LoadInt32(&h.closed) == 1
}

func (h *heapTimer) exec(fn func()) {
	gopool.Execute(fn)
}

func (h *heapTimer) addTask(t *taskpool.Task) {
	// 轮询的往bucket中添加延时任务
	h.bucketPos = (h.bucketPos + 1) % bucketLen
	bkt := h.timeBuckets[h.bucketPos]
	bkt.add(t)
	h.taskMap.Store(t.ID, h.bucketPos)
}

func (h *heapTimer) deleteTask(taskId int64) {
	v, exist := h.taskMap.Load(taskId)
	if !exist {
		return
	}
	h.timeBuckets[v.(int)].delete(taskId)
	h.taskMap.Delete(taskId)
}

func (h *heapTimer) After(delay time.Duration, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if h.isClosed() {
		return timer.EmptyTaskID, timer.ErrTimerClosed
	}

	t := taskpool.Get(delay, fn, false, nil)
	gopool.Execute(func() {
		h.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}

func (h *heapTimer) Every(delay time.Duration, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if h.isClosed() {
		return timer.EmptyTaskID, timer.ErrTimerClosed
	}

	t := taskpool.Get(delay, fn, true, nil)
	gopool.Execute(func() {
		h.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}

func (h *heapTimer) Delete(id timer.TaskID) error {
	if id == timer.EmptyTaskID {
		return nil
	}
	if h.isClosed() {
		return timer.ErrTimerClosed
	}
	gopool.Execute(func() {
		h.bufferChan <- id
	})
	return nil
}

func (h *heapTimer) Stop() {
	if !atomic.CompareAndSwapInt32(&h.closed, 0, 1) {
		return
	}
	h.cancel()
	// 释放协程池
	gopool.Release()

	n := 0
	h.taskMap.Range(func(key, value any) bool {
		n += 1
		return true
	})
}

func (h *heapTimer) Cron(desc string, fn func()) (timer.TaskID, error) {
	if fn == nil {
		return timer.EmptyTaskID, timer.ErrNilFunc
	}
	if h.isClosed() {
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
		return timer.EmptyTaskID, err
	}
	var t = taskpool.Get(nextTime.Sub(now), fn, false, expr)
	gopool.Execute(func() {
		h.bufferChan <- t
	})
	return timer.TaskID(t.ID), nil
}
