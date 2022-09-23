package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/pyihe/go-pkg/syncs"
	"github.com/pyihe/timer"
	"github.com/pyihe/timer/timewheel"
)

func main() {
	var tmr timer.Timer

	defer func() {
		if tmr != nil {
			tmr.Stop()
			time.Sleep(5 * time.Second)
		}
	}()

	tmr = timewheel.New(10*time.Millisecond, 256, 16)
	//tmr = timeheap.New(16)

	//testAfter(tmr)
	testEvery(tmr)
}

func testAfter(tmr timer.Timer) {
	counter := new(syncs.AtomicInt64)
	m := sync.Map{}
	for i := 1; i <= 10000; i++ {
		go func(d int) {
			id, _ := tmr.After(time.Duration(d)*time.Millisecond, func() {
				//fmt.Println("xxx: ", d)
				counter.Inc(1)
			})
			m.Store(id, struct{}{})
		}(i)
	}

	time.Sleep(30 * time.Second)
	fmt.Println("counter = ", counter.Value())

	m.Range(func(key, value any) bool {
		tmr.Delete(key.(int64))
		return true
	})

	time.Sleep(10 * time.Second)
}

func testEvery(tmr timer.Timer) {
	counter := new(syncs.AtomicInt64)
	m := sync.Map{}
	for i := 1; i <= 10000; i++ {
		go func(d int) {
			id, _ := tmr.Every(time.Duration(d)*time.Millisecond, func() {
				//fmt.Println("xxx", d)
				counter.Inc(1)
			})
			m.Store(id, struct{}{})
		}(i)
	}

	time.Sleep(30 * time.Second)
	fmt.Println("counter = ", counter.Value())
	m.Range(func(key, value any) bool {
		tmr.Delete(key.(int64))
		return true
	})
	time.Sleep(10 * time.Second)
}
