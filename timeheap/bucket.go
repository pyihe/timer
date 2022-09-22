package timeheap

import (
	"time"

	"gosample/times/timeheap/heaps"
)

type task struct {
	deadline time.Time     // 最近一次执行的截止时间
	delay    time.Duration // 任务延时
	id       int64         // 任务ID
	fn       func()        // 任务对应的函数
	index    int           // 任务索引
	repeated bool          // 是否重复执行
}

func (t *task) reset() {
	*t = task{
		index: -1,
	}
}

type bucket []*task

func newBucket(c int) *bucket {
	b := make(bucket, 0, c)
	return &b
}

func (b *bucket) Len() int {
	return len(*b)
}

func (b *bucket) Less(i, j int) bool {
	return (*b)[i].deadline.Before((*b)[j].deadline)
}

func (b *bucket) Swap(i, j int) {
	(*b)[i].index = j
	(*b)[j].index = i
	(*b)[i], (*b)[j] = (*b)[j], (*b)[i]
}

func (b *bucket) Push(x interface{}) {
	t, ok := x.(*task)
	if !ok {
		return
	}
	n := len(*b)
	c := cap(*b)
	// 需要扩容
	if n+1 > c {
		nb := make(bucket, n, c*2)
		copy(nb, *b)
		*b = nb
	}
	*b = (*b)[0 : n+1]
	(*b)[n] = t
	t.index = n
}

func (b *bucket) Pop() interface{} {
	n := len(*b)
	c := cap(*b)
	if n < (c/2) && c > 25 {
		nb := make(bucket, n, c/2)
		copy(nb, *b)
		*b = nb
	}
	if n == 0 {
		return nil
	}
	x := (*b)[n-1]
	(*b)[n-1] = nil
	*b = (*b)[:n-1]
	return x
}

func (b *bucket) peek() *task {
	if len(*b) == 0 {
		return nil
	}

	return (*b)[0]
}

func (b *bucket) fix(t *task) {
	heaps.Fix(b, t.index)
}

func (b *bucket) delete(i int) {
	heaps.Remove(b, i)
}

func (b *bucket) add(t *task) {
	heaps.Push(b, t)
}
