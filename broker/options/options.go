package options

// Options configures how the broker behaves when enqueuing or scheduling a task.
type Options struct {
	// If true, only one task with the same kind is allowed to be enqueued or scheduled.
	// When specifying this option, if there already exists a pending or scheduled task
	// with the same kind an error is returned and the task is not enqueued or scheduled.
	OnlyOne bool

	// If true, only one task with the same kind or kind aliases is allowed to be
	// enqueued or scheduled. When specifying this option, if there already are tasks
	// of the same kind they are cancelled and replaced by this new task.
	OnlyOneReplace bool
}
