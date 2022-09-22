package heaptimer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyihe/timer"

	"github.com/panjf2000/ants/v2"
	"github.com/pyihe/go-pkg/snowflakes"
)

const bucketLen = 64

type asynHandler func(func())

type heapTimer struct {
	cancel      context.CancelFunc     // 停止所有协程（不包括执行任务的协程）
	idGenerator snowflakes.Worker      // 任务ID生成器
	gPool       *ants.Pool             // 协程池
	taskPool    *sync.Pool             // 任务变量池，防止频繁分配内存
	taskMap     *sync.Map              // key: task.id	value: bucket index
	timeBuckets [bucketLen]*timeBucket // bucket
	bufferChan  chan interface{}       // 用于添加、删除任务的通道
	recycleChan chan *task             // 用于回收task变量的通道
	closed      int32                  // timer是否关闭
	bucketPos   int                    // bucket索引
}

func New(options ...timer.Option) timer.Timer {
	var err error
	var ctx context.Context
	var opts = &timer.Options{
		Node:         1,
		GoPoolSize:   1000,
		TaskChanSize: 1024,
	}
	var h = &heapTimer{
		taskPool: &sync.Pool{
			New: func() interface{} {
				return &task{}
			},
		},
		taskMap:     &sync.Map{},
		timeBuckets: [64]*timeBucket{},
		recycleChan: make(chan *task, bucketLen),
		closed:      0,
		bucketPos:   -1,
	}

	for _, op := range options {
		op(opts)
	}
	ctx, h.cancel = context.WithCancel(context.Background())
	h.idGenerator = snowflakes.NewWorker(opts.Node)
	h.bufferChan = make(chan interface{}, opts.TaskChanSize)
	h.gPool, err = ants.NewPool(opts.GoPoolSize, ants.WithNonblocking(true))
	if err != nil {
		panic(err)
	}

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
		_ = h.gPool.Submit(func() {
			tb.start(ctx)
		})
	}

	_ = h.gPool.Submit(func() {
		h.run(ctx)
	})

	_ = h.gPool.Submit(func() {
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
				h.taskMap.Delete(t.id)
				h.putTask(t)
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
			case *task:
				h.addTask(t.(*task))
			case int64:
				fmt.Println("delete int64")
				h.deleteTask(t.(int64))
			}
		}
	}
}

func (h *heapTimer) isClosed() bool {
	return atomic.LoadInt32(&h.closed) == 1
}

func (h *heapTimer) getTask() *task {
	t, ok := h.taskPool.Get().(*task)
	if ok {
		return t
	}
	return &task{}
}

func (h *heapTimer) putTask(t *task) {
	if t == nil {
		return
	}
	t.reset()
	h.taskPool.Put(t)
}

func (h *heapTimer) exec(fn func()) {
	_ = h.gPool.Submit(func() {
		fn()
	})
}

func (h *heapTimer) addTask(t *task) {
	// 轮询的往bucket中添加延时任务
	h.bucketPos = (h.bucketPos + 1) % bucketLen
	bkt := h.timeBuckets[h.bucketPos]
	bkt.add(t)
	h.taskMap.Store(t.id, h.bucketPos)
}

func (h *heapTimer) deleteTask(taskId int64) {
	v, exist := h.taskMap.Load(taskId)
	if !exist {
		return
	}
	h.timeBuckets[v.(int)].delete(taskId)
	h.taskMap.Delete(taskId)
}

func (h *heapTimer) After(delay time.Duration, fn func()) (int64, error) {
	if h.isClosed() {
		return 0, timer.ErrTimerClosed
	}
	t := h.getTask()
	t.delay = delay
	t.fn = fn
	t.id = h.idGenerator.GetInt64()
	t.repeated = false

	err := h.gPool.Submit(func() {
		h.bufferChan <- t
	})

	return t.id, err
}

func (h *heapTimer) Every(delay time.Duration, fn func()) (int64, error) {
	if h.isClosed() {
		return 0, timer.ErrTimerClosed
	}

	t := h.getTask()
	t.delay = delay
	t.fn = fn
	t.id = h.idGenerator.GetInt64()
	t.repeated = true

	err := h.gPool.Submit(func() {
		h.bufferChan <- t
	})

	return t.id, err
}

func (h *heapTimer) Delete(id int64) error {
	if h.isClosed() {
		return timer.ErrTimerClosed
	}
	return h.gPool.Submit(func() {
		h.bufferChan <- id
	})
}

func (h *heapTimer) Stop() {
	if !atomic.CompareAndSwapInt32(&h.closed, 0, 1) {
		return
	}
	h.cancel()
	// 释放协程池
	h.gPool.ReleaseTimeout(5 * time.Second)
}
