package times

import (
	"time"

	"github.com/pyihe/go-pkg/errors"
)

type Timer interface {
	Stop()                                                      // 停止定时器
	Delete(taskID int64) error                                  // 删除指定ID的任务
	After(d time.Duration, fn func()) (taskID int64, err error) // duration后执行一次, 返回任务ID
	Every(d time.Duration, fn func()) (taskID int64, err error) // 每duration执行一次, 返回任务ID
}

var (
	ErrTimerClosed = errors.New("timer closed")
)
