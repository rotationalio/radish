package options

import (
	"database/sql"
	"fmt"
	"strings"
)

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

	// Kinds to match OnlyOne or OnlyOneReplace against.
	// This is generally not specified by the user.
	Kinds []string
}

func (o *Options) KindsSQLite3Params() (clause string, params []any) {
	params = make([]any, 0, len(o.Kinds))
	placeholders := make([]string, 0, len(o.Kinds))

	for i, kind := range o.Kinds {
		placeholder := fmt.Sprintf("kind%d", i+1)
		placeholders = append(placeholders, ":"+placeholder)
		params = append(params, sql.Named(placeholder, kind))
	}

	clause = "(" + strings.Join(placeholders, ",") + ")"
	return clause, params
}
