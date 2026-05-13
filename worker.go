package radish

import (
	"context"
	"fmt"
	"time"

	"go.rtnl.ai/radish/internal/worker"
)

//============================================================================
// Worker Interface
//============================================================================

// All workers must implement the Worker interface and follow the concurrency
// guidelines in order to process tasks of the given type.
type Worker[T Task] interface {
	// By implementing this interface, the worker is able to determine if the task
	// should be retried for the given task info, and how long to delay before the
	// next retry. If nil is returned as the retry, the default retry policy is used.
	Retry(*TaskInfo[T]) *Retry

	// By implementing this interface, the worker is able to determine the timeout for the
	// given task. If 0 is returned as the timeout, the default timeout policy is used.
	Timeout(*TaskInfo[T]) time.Duration

	// Do performs the work for for the given task and returns an error if the task
	// fails. The context will be configured with a timeout according to the worker
	// settings and may be cancelled for other reasons.
	//
	// If no error is returned, the job is assumed to have succeeded and will be marked
	// as completed. Any error returned will be part of the task info saved in the backend.
	//
	// Note that the Do method is expected to be thread-safe and may be called
	// concurrently by multiple workers. Depending on the global concurrency settings.
	Do(context.Context, *TaskInfo[T]) error
}

// Indicates if the task should be retried and the delay before the next retry. If no
// delay is provided, the default delay will be used.
type Retry struct {
	Retry bool
	Delay time.Duration
}

//============================================================================
// Worker Defaults for Embedding
//============================================================================

// Embed this struct in a worker that only implements the Do method to provide the
// default retry and timeout policies.
type WorkerDefaults[T Task] struct{}

func (w *WorkerDefaults[T]) Retry(*TaskInfo[T]) *Retry { return nil }

func (w *WorkerDefaults[T]) Timeout(*TaskInfo[T]) time.Duration { return 0 }

//============================================================================
// Workers as functions (for quick worker definitions)
//============================================================================

// Wrap a function to implement the Worker interface. A Task is required to specify
// the kind of task that the function will process.
func WorkFunc[T Task](do func(context.Context, *TaskInfo[T]) error) Worker[T] {
	return &workFunc[T]{
		kind: (*new(T)).Kind(),
		do:   do,
	}
}

// Function wrapper that implements the Worker interface.
type workFunc[T Task] struct {
	WorkerDefaults[T]

	kind string
	do   func(context.Context, *TaskInfo[T]) error
}

func (wf *workFunc[T]) Do(ctx context.Context, task *TaskInfo[T]) error {
	return wf.do(ctx, task)
}

//============================================================================
// Workers Registration and Wrapper
//============================================================================

func Register[T Task](r *Radish, worker Worker[T]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRunning() {
		return ErrRunning
	}
	return AddWorkerSafe(r.workers, worker)
}

func MustRegister[T Task](r *Radish, worker Worker[T]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRunning() {
		panic(ErrRunning)
	}
	AddWorker(r.workers, worker)
}

func AddWorker[T Task](w *Workers, worker Worker[T]) {
	if err := AddWorkerSafe(w, worker); err != nil {
		panic(err)
	}
}

func AddWorkerSafe[T Task](w *Workers, worker Worker[T]) error {
	var task T
	return w.add(task, &workerFactory[T]{worker: worker})
}

// Workers is a list of available job workers. A worker must be registered for each
// type of task that can be handled by the radish instance.
type Workers struct {
	workers map[string]untypedWorker
}

type untypedWorker struct {
	task    Task
	factory worker.Factory
}

func (w *Workers) add(task Task, factory worker.Factory) error {
	checkRegistered := func(kind string) error {
		if _, ok := w.workers[kind]; ok {
			return fmt.Errorf("task %q is already registered", kind)
		}
		return nil
	}

	if w.workers == nil {
		w.workers = make(map[string]untypedWorker)
	}

	kind := task.Kind()
	if err := checkRegistered(kind); err != nil {
		return err
	}

	w.workers[kind] = untypedWorker{
		task:    task,
		factory: factory,
	}

	if aliases, ok := task.(TaskWithAliases); ok {
		for _, alias := range aliases.KindAliases() {
			if err := checkRegistered(alias); err != nil {
				return err
			}
			w.workers[alias] = w.workers[kind]
		}
	}

	return nil
}

func (w *Workers) Has(kind string) bool {
	_, ok := w.workers[kind]
	return ok
}

func (w *Workers) Len() int {
	return len(w.workers)
}
