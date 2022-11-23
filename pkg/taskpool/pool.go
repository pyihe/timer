package taskpool

import (
	"sync"
	"time"

	"github.com/pyihe/go-pkg/snowflakes"
	"github.com/pyihe/timer/pkg/cronexpr"
)

type Task struct {
	ID       int64              // 任务ID
	Delay    time.Duration      // 任务延时
	Job      func()             // 任务执行内容
	Extra    [2]interface{}     // 额外的任务属性
	Repeated bool               // 是否重复执行
	Expr     *cronexpr.CronExpr // desc
}

var (
	pool        sync.Pool
	idGenerator = snowflakes.NewWorker(0)
)

func Get(delay time.Duration, job func(), repeated bool, cronExpr *cronexpr.CronExpr) (t *Task) {
	v := pool.Get()
	if v == nil {
		t = &Task{}
	} else {
		t = v.(*Task)
	}

	t.ID = idGenerator.GetInt64()
	t.Delay = delay
	t.Job = job
	t.Repeated = repeated
	t.Expr = cronExpr
	return
}

func Put(t *Task) {
	if t == nil {
		return
	}
	*t = Task{}
	pool.Put(t)
}
