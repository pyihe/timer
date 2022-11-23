package main

import (
	"fmt"
	"time"

	"github.com/pyihe/timer/timewheel"
)

func main() {
	tmr := timewheel.New(10*time.Millisecond, 256, 16)
	defer tmr.Stop()

	afterID, err := tmr.After(1*time.Second, func() {
		fmt.Println("after 1 second")
	})
	if err != nil {
		//handle err
	}

	// operate taskId if necessary
	_ = afterID

	everyID, err := tmr.Every(1*time.Second, func() {
		fmt.Println("every 1 second")
	})
	if err != nil {
		//handle err
	}

	// operate taskId if necessary
	_ = everyID
}
