package main

import (
	"fmt"
	"time"

	"github.com/pyihe/go-pkg/syncs"
	"github.com/pyihe/timer/heaptimer"
)

func main() {
	timer := heaptimer.New()
	defer timer.Stop()

	begin := time.Now()
	counter := new(syncs.AtomicInt64)

	for i := 10; i < 10000; i++ {
		go func(idx int) {
			timer.After(time.Duration(idx)*time.Millisecond, func() {
				counter.Inc(1)
				fmt.Println("after", idx, time.Now().Sub(begin))
			})
		}(i)
	}

	//for i := 100; i < 10000; i++ {
	//	go func(idx int) {
	//		everyID, _ := timer.Every(time.Duration(idx)*time.Millisecond, func() {
	//			fmt.Println("every", idx, time.Now().Sub(begin))
	//		})
	//		time.Sleep(1 * time.Second)
	//		timer.Delete(everyID)
	//	}(i)
	//}
	time.Sleep(15 * time.Second)
	fmt.Println(counter.Value())
	select {}
}
