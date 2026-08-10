package radish

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"go.rtnl.ai/radish/models"
)

// ErrorHandler handles a runtime executor error. The task is nil when the
// error occurs while dequeuing; otherwise it contains the task being worked.
// The context is detached from the worker task context and has the configured
// cleanup timeout. The handler runs on the executor goroutine and should
// not synchronously call Shutdown.
type ErrorHandler func(context.Context, *models.TaskMeta, error)

var (
	ErrNoDatabase = errors.New("no database connection or configuration provided")
	ErrRunning    = errors.New("radish cannot be modified while running")
	ErrStop       = errors.New("stop signal received")
)

type PanicRecovery struct {
	Err   error
	Trace string
}

func (p *PanicRecovery) Error() string {
	return p.Err.Error()
}

func (p *PanicRecovery) Unwrap() error {
	return p.Err
}

func Recover(r any) *PanicRecovery {
	var err error

	switch x := r.(type) {
	case error:
		err = x
	case string:
		err = errors.New(x)
	case nil:
		err = errors.New("nil panic")
	default:
		err = fmt.Errorf("panic: %v", r)
	}

	return &PanicRecovery{
		Err:   err,
		Trace: string(debug.Stack()),
	}
}

func AddError(task *models.TaskMeta, err error) {
	var recovered *PanicRecovery
	if errors.As(err, &recovered) {
		task.AddError(recovered.Err, recovered.Trace)
	} else {
		task.AddError(err, "")
	}
}
