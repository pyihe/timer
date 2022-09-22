package main

import (
	"fmt"
	"time"

	"github.com/pyihe/timer/wheeltimer"
)

func main() {
	testWheelAfter()
}

func testWheelAfter() {
	timer := wheeltimer.New(10*time.Millisecond, 256)

	for i := 11; i <= 10000; i++ {
		go func(d int) {
			timer.After(time.Duration(d)*time.Millisecond, func() {
				fmt.Println("xxx: ", d)
			})
		}(i)
	}

	time.Sleep(30 * time.Second)
	timer.Stop()
	time.Sleep(1 * time.Second)
}
