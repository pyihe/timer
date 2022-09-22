package timer

type Options struct {
	Node         int64 // 服务节点ID
	GoPoolSize   int   // 协程池容量
	TaskChanSize int   // 任务通道缓冲区大小
}

type Option func(*Options)

// WithNode 如果任务需要本地化，为了任务ID冲突，通过服务节点ID来区分
// 最多支持1024个节点：[0, 1023]
func WithNode(node int64) Option {
	return func(options *Options) {
		options.Node = node
	}
}

// WithGoPoolSize 初始化协程池的数量：异步执行任务，同时任务执行依托于协程池数量
func WithGoPoolSize(size int) Option {
	return func(options *Options) {
		options.GoPoolSize = size
	}
}

// WithChannelBufferSize 添加/删除任务的通道缓冲区大小，任务添加/删除公用一个通道
// 如果同时添加的任务数量很多，尽量将缓冲区设置大一点
func WithChannelBufferSize(size int) Option {
	return func(options *Options) {
		options.TaskChanSize = size
	}
}
