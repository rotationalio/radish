package radish

import "go.rtnl.ai/radish/broker/options"

// Specify options to the broker when enqueuing or scheduling a task.
type Option func(opts *options.Options)

// If true, only one task with the same kind or kind alias is allowed to be enqueued or
// scheduled. When specifying this option, if there already exists a pending, running,
// or scheduled task with the same kind an error is returned and the task is not
// enqueued or scheduled.
func OnlyOne() Option {
	return func(opts *options.Options) {
		opts.OnlyOne = true
	}
}

// If true, only one task with the same kind or kind alias is allowed to be enqueued or
// scheduled. When specifying this option, if there already exists a pending or
// scheduled task with the same kind they are cancelled and replaced by this new task.
//
// NOTE: this option does not cancel running tasks that are currently in progress since
// there currently is no way to cancel an in-progress task. That means there is an edge
// case where two tasks of the same kind that can exist using OnlyOneReplace: if the
// running task fails and is then set to retry, the retry task and this new task will
// both exist. To avoid this case, it is recommended to use OnlyOneReplace with a retry
// policy of one attempt only (e.g. no retries).
func OnlyOneReplace() Option {
	return func(opts *options.Options) {
		opts.OnlyOneReplace = true
	}
}
