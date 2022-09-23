package timeheap

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/pyihe/timer/pkg/taskpool"
)

type timeBucket struct {
	mu            sync.RWMutex             // guard below
	b             *bucket                  // 任务列表
	ticker        *time.Ticker             // 定时器
	taskMap       map[int64]*taskpool.Task // 任务
	heapNotify    chan struct{}            // 重新堆化时需要通知重新获取堆顶元素
	recycleNotify chan *taskpool.Task      // 回收任务
	asynExec      asynHandler              // 执行任务的handler
}

func newTimeBucket(recycleChan chan *taskpool.Task, exec asynHandler) *timeBucket {
	tb := &timeBucket{
		mu:            sync.RWMutex{},
		b:             newBucket(bucketLen), // bucket初始容量设置为64
		taskMap:       make(map[int64]*taskpool.Task),
		heapNotify:    make(chan struct{}, 1),
		asynExec:      exec,
		recycleNotify: recycleChan,
	}
	return tb
}

func (tb *timeBucket) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			if tb.ticker != nil {
				tb.ticker.Stop()
			}
			return
		default:
			tb.tick()
		}
	}
}

func (tb *timeBucket) tick() {
	const maxDuration = time.Duration(math.MaxInt32) * time.Second

	var duration time.Duration

	tb.mu.RLock()
	t := tb.b.peek()
	tb.mu.RUnlock()

	duration = maxDuration
	if t != nil {
		deadline := t.Extra[0].(time.Time)
		duration = deadline.Sub(time.Now())
	}
	// 高并发情况下，定时器被 heapNotify 中断，再次执行时，任务已经过期，这时duration为负数，
	// 当duration为负数时不应依赖于ticker，直接执行任务
	if duration <= 0 {
		tb.runTask(t)
		return
	}
	if tb.ticker == nil {
		tb.ticker = time.NewTicker(duration)
	} else {
		tb.ticker.Reset(duration)
	}
	select {
	case <-tb.heapNotify:
		tb.ticker.Stop()
		break
	case <-tb.ticker.C:
		tb.runTask(t)
	}
}

func (tb *timeBucket) runTask(t *taskpool.Task) {
	// 执行任务
	job := t.Job
	tb.asynExec(job)

	index := t.Extra[1].(int)
	// 删除任务
	tb.mu.Lock()
	tb.b.delete(index)
	tb.mu.Unlock()

	// 如果是重复执行，则再次添加
	if t.Repeated {
		tb.add(t)
	} else {
		// 否则从任务列表中删除
		tb.mu.Lock()
		delete(tb.taskMap, t.ID)
		tb.mu.Unlock()
		// 同时回收任务变量
		tb.asynExec(func() {
			tb.recycleNotify <- t
		})
	}
}

func (tb *timeBucket) add(t *taskpool.Task) {
	// 更新截止时间
	t.Extra[0] = time.Now().Add(t.Delay)

	tb.mu.Lock()
	tb.b.add(t)
	tb.taskMap[t.ID] = t
	tb.mu.Unlock()

	// 通知tick，有新的任务来了，需要重新找延时最少的任务
	// 防止阻塞，异步通知
	tb.asynExec(func() {
		tb.heapNotify <- struct{}{}
	})
}

func (tb *timeBucket) delete(id int64) {
	tb.mu.Lock()
	t, exist := tb.taskMap[id]
	if !exist {
		tb.mu.Unlock()
		return
	}
	tb.b.delete(t.Extra[1].(int))
	delete(tb.taskMap, id)
	tb.mu.Unlock()

	// 防止阻塞，异步通知
	tb.asynExec(func() {
		tb.heapNotify <- struct{}{}
	})
}
