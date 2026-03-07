package radish

import "go.rtnl.ai/radish/models"

// Task is an interface that represents the arguments for the execution of a specifc
// task of type T that is processed by a worker that also implements type T. This
// data structure is serialized to JSON and sent to the broker for processing.
//
// The struct is serialized using `encoding/json` and respects json struct tags.
type Task interface {
	// Kind returns the unique identifier for the task type. This is used to route the
	// task to the appropriate worker. We use strings rather than type names to allow
	// application specific definitions and renaming of task types.
	//
	// Kinds should be formatted without spaces and generally lowercase. Avoid using
	// special characters and use only alphanumeric characters.
	//
	// After deploying tasks, it is not safe to rename the kind (unless the queue has
	// been cleared of all existing tasks). Renaming safely can be accomplished using
	// the `TaskWithAliases` interface.
	Kind() string
}

// TaskWithAliases is an interface that allows for the definition of aliases for a task
// kind. This is useful for renaming tasks after deployment without losing existing tasks.
type TaskWithAliases interface {
	// Kinds that the associated task worker will respond to.
	KindAliases() []string
}

type TaskInfo[T Task] struct {
	*models.TaskMeta

	Task T
}
