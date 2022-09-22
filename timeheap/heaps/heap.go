package heaps

import "sort"

// 四叉堆

type Interface interface {
	sort.Interface
	Push(interface{})
	Pop() interface{}
}

func Init(h Interface) {
	n := h.Len()
	for i := n/4 - 1; i >= 0; i-- {
		down(h, i, n)
	}
}

func Push(h Interface, x interface{}) {
	h.Push(x)
	up(h, h.Len()-1)
}

func Pop(h Interface) interface{} {
	n := h.Len() - 1
	h.Swap(0, n)
	down(h, 0, n)
	return h.Pop()
}

func Remove(h Interface, i int) interface{} {
	n := h.Len() - 1
	if n != i {
		h.Swap(i, n)
		if !down(h, i, n) {
			up(h, i)
		}
	}
	return h.Pop()
}

func Fix(h Interface, i int) {
	if !down(h, i, h.Len()) {
		up(h, i)
	}
}

// 子节点上升
func up(h Interface, j int) {
	for {
		i := (j - 1) / 4
		if i == j || !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		j = i
	}
}

// 父节点下沉
func down(h Interface, i0, n int) bool {
	i := i0
outLoop:
	for {
		// find the biggest or smallest leaf from parent
		j := 4*i + 1 // first child
		for k := 1; k <= 4; k++ {
			jj := 4*i + k
			if jj >= n || jj < 0 {
				break outLoop
			}
			if h.Less(jj, j) {
				j = jj
			}
		}
		if !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		i = j
	}
	return i > i0
}
