package worker

import (
	"context"
	"time"

	"go.rtnl.ai/radish/models"
)

type Worker interface {
	Retry() *Retry
	Timeout() time.Duration
	UnmarshalTask() error
	Do(ctx context.Context) error
}

type Factory interface {
	Make(task *models.TaskMeta) (Worker, error)
}

// An internal clone of the Retry struct from the radish package for type indirection.
type Retry struct {
	Retry bool
	Delay time.Duration
}
