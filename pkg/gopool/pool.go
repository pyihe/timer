package gopool

import (
	"log"

	"github.com/panjf2000/ants/v2"
)

var (
	pool *ants.Pool
)

func init() {
	var err error
	pool, err = ants.NewPool(ants.DefaultAntsPoolSize, ants.WithNonblocking(true))
	if err != nil {
		log.Fatalln(err)
	}
}

func Release() {
	if pool == nil {
		log.Fatalln("uninitialized pool")
	}
	pool.Release()
}

func Execute(fn func()) {
	if pool == nil {
		log.Fatalln("uninitialized pool")
	}
	if pool.IsClosed() {
		pool.Reboot()
	}
	if err := pool.Submit(fn); err != nil {
		log.Println("submit task fail: ", err)
	}
}
