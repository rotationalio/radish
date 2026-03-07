package radish

import (
	"context"
	"encoding/json"
	"time"

	"go.rtnl.ai/radish/internal/worker"
	"go.rtnl.ai/radish/models"
)

// The worker factory implements the worker.Factory interface to create a wrapped worker.
// This converts the generic Worker[T] interface to the worker.Worker interface.
type workerFactory[T Task] struct {
	worker Worker[T]
}

// The wrapped worker implements the worker.Worker interface to perform the actual work.
// This converts the generic Worker[T] interface to the worker.Worker interface.
type wrappedWorker[T Task] struct {
	task   *TaskInfo[T]
	meta   *models.TaskMeta
	worker Worker[T]
}

// The worker factory creates a wrapped worker for the given task.
func (f *workerFactory[T]) Make(task *models.TaskMeta) (worker.Worker, error) {
	return &wrappedWorker[T]{
		meta:   task,
		worker: f.worker,
	}, nil
}

// Retry passes through to the typed underlying worker.
func (w *wrappedWorker[T]) Retry() *worker.Retry {
	if retry := w.worker.Retry(w.task); retry != nil {
		return &worker.Retry{Retry: retry.Retry, Delay: retry.Delay}
	}
	return nil
}

// Timeout passes through to the typed underlying worker.
func (w *wrappedWorker[T]) Timeout() time.Duration {
	return w.worker.Timeout(w.task)
}

// Create the typed task info then unmarshal the task payload into the task.
// This must be called before the Do method otherwise the task will be nil.
func (w *wrappedWorker[T]) UnmarshalTask() error {
	w.task = &TaskInfo[T]{
		TaskMeta: w.meta,
	}
	return json.Unmarshal(w.meta.Payload, &w.task.Task)
}

// Do passes through to the typed underlying worker.
func (w *wrappedWorker[T]) Do(ctx context.Context) error {
	return w.worker.Do(ctx, w.task)
}
