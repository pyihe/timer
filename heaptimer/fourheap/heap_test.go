package fourheap

import (
	"fmt"
	"testing"
)

type Slice []int

func (s *Slice) String() string {
	return fmt.Sprint(*s)
}

func (s *Slice) Len() int {
	return len(*s)
}

func (s *Slice) Less(i, j int) bool {
	return (*s)[i] < (*s)[j]
}

func (s *Slice) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

func (s *Slice) Push(x interface{}) {
	v, ok := x.(int)
	if !ok {
		return
	}
	*s = append(*s, v)
}

func (s *Slice) Pop() (v interface{}) {
	if n := s.Len(); n > 0 {
		v = (*s)[n-1]
		*s = (*s)[:n-1]
	}
	return
}

var data = &Slice{
	10, 3, 5, 2, 1, 9, 8, 7, 4, 6,
}

func TestInit(t *testing.T) {
	Init(data)
	t.Logf("after init: %v\n", data)
}

func TestPush(t *testing.T) {
	Push(data, -1)
	t.Logf("after push: %v\n", data)
}

func TestPop(t *testing.T) {
	v := Pop(data)
	t.Logf("after pop: %v, %v\n", v, data)
}

func TestRemove(t *testing.T) {
	v := Remove(data, 5)
	t.Logf("after remove: %v, %v\n", v, data)
}

func TestFix(t *testing.T) {
	(*data)[0] = 20
	Fix(data, 0)
	t.Logf("after fix: %v\n", data)
}

func BenchmarkPush(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Push(data, 1)
	}
}

func BenchmarkRemove(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Push(data, Remove(data, 0))
	}
}
