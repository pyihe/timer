package timer

import (
	"time"

	"github.com/pyihe/go-pkg/errors"
)

const (
	EmptyTaskID TaskID = 0
)

var (
	ErrTimerClosed = errors.New("timer closed")
	ErrInvalidExpr = errors.New("invalid cron desc")
)

type TaskID int64

func (t TaskID) Int64() int64 {
	return int64(t)
}

type Timer interface {
	Stop()                                                       // 停止定时器
	Delete(taskID TaskID) error                                  // 删除指定ID的任务
	After(d time.Duration, fn func()) (taskID TaskID, err error) // duration后执行一次, 返回任务ID
	Every(d time.Duration, fn func()) (taskID TaskID, err error) // 每duration执行一次, 返回任务ID
	Cron(desc string, fn func()) (taskID TaskID, err error)      // 按照给定的DESC执行任务
}
