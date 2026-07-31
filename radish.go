package radish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.rtnl.ai/radish/backoff"
	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/radish/broker/cursor"
	dberr "go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/broker/options"
	"go.rtnl.ai/radish/broker/postgres"
	internal "go.rtnl.ai/radish/internal/worker"
	"go.rtnl.ai/radish/jitter"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/rlog"
)

type Radish struct {
	mu        sync.RWMutex
	wg        *sync.WaitGroup
	conf      Config
	workers   *Workers
	executors []*executor
	vacuum    *vacuum
	broker    broker.Broker
}

type executor struct {
	conf    *Config
	workers *Workers
	broker  broker.Broker
	backoff backoff.BackOff
	stop    chan<- struct{}
}

func New(conf *Config) (_ *Radish, err error) {
	// If no config is provided, load the config from the environment.
	// Otherwise, validate the config in case it wasn't loaded from the environment.
	if conf == nil {
		var cfg Config
		if cfg, err = LoadConfig(); err != nil {
			return nil, err
		}
		conf = &cfg
	} else {
		if err = conf.Validate(); err != nil {
			return nil, err
		}
	}

	// If using managed database, validate that a connection is provided.
	if conf.ManagedDB && conf.Conn == nil {
		return nil, ErrNoDatabase
	}

	return &Radish{
		conf:      *conf,
		executors: nil,
		workers: &Workers{
			workers: make(map[string]untypedWorker),
		},
	}, nil
}

func (r *Radish) Register(worker Worker[Task]) error {
	return AddWorkerSafe(r.workers, worker)
}

// Starts the radish executors each in their own goroutine with a copy of the config
// and workers to execute tasks in parallel. Returns an error if radish is already running.
func (r *Radish) Run() (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRunning() {
		return ErrRunning
	}

	// Create a backoff policy for the executors.
	var policy backoff.BackOff
	if policy, err = backoff.New(r.conf.Backoff); err != nil {
		return err
	}

	// Connect to the database or use the provided database connection.
	if r.conf.ManagedDB {
		if r.conf.Conn == nil {
			return ErrNoDatabase
		}

		// TODO: allow using other database connection types besides postgres.
		if r.broker, err = postgres.Use(r.conf.Conn); err != nil {
			return err
		}
	} else {
		if r.broker, err = broker.Connect(r.conf.DatabaseURL); err != nil {
			return err
		}
	}

	// Create a wait group to wait for all executors to finish.
	r.wg = &sync.WaitGroup{}
	r.wg.Add(r.conf.NumWorkers)

	// Create the executors with a copy of the config and workers to execute tasks in parallel.
	for i := 0; i < r.conf.NumWorkers; i++ {
		executor := &executor{conf: &r.conf, workers: r.workers, broker: r.broker, backoff: policy}
		r.executors = append(r.executors, executor)
		executor.run(r.wg)
	}

	// Start the vacuum background task.
	if r.conf.Retention > 0 && r.conf.VacuumInterval > 0 {
		r.wg.Add(1)
		r.vacuum = &vacuum{conf: &r.conf, broker: r.broker}
		r.vacuum.run(r.wg)
	}
	return nil
}

// Shuts down the radish executors and waits for them to finish processing all tasks.
func (r *Radish) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRunning() {
		return
	}

	// Signal the vacuum background task to stop.
	if r.vacuum != nil {
		r.vacuum.shutdown()
		r.vacuum = nil
	}

	// Signal all executors to stop.
	for _, executor := range r.executors {
		executor.shutdown()
	}

	// Wait until all executors have finished.
	r.wg.Wait()

	r.executors = nil
	r.wg = nil

	// Shutdown the broker.
	if r.broker != nil {
		if err := r.broker.Close(); err != nil {
			rlog.Error("could not close broker", "error", err)
		}
	}
}

func (r *Radish) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isRunning()
}

func (r *Radish) isRunning() bool {
	return len(r.executors) > 0 || r.vacuum != nil
}

func (r *Radish) Enqueue(ctx context.Context, task Task, opts ...Option) (id int64, err error) {
	var data []byte
	if data, err = json.Marshal(task); err != nil {
		return 0, fmt.Errorf("could not marshal task json: %w", err)
	}

	var brokerOptions *options.Options
	if len(opts) > 0 {
		brokerOptions = &options.Options{}
		for _, opt := range opts {
			opt(brokerOptions)
		}
	}

	return r.broker.Enqueue(ctx, task.Kind(), data, brokerOptions)
}

func (r *Radish) Schedule(ctx context.Context, task Task, executeAfter time.Time, opts ...Option) (id int64, err error) {
	var data []byte
	if data, err = json.Marshal(task); err != nil {
		return 0, fmt.Errorf("could not marshal task json: %w", err)
	}

	var brokerOptions *options.Options
	if len(opts) > 0 {
		brokerOptions = &options.Options{}
		for _, opt := range opts {
			opt(brokerOptions)
		}
	}

	return r.broker.Schedule(ctx, task.Kind(), data, executeAfter, brokerOptions)
}

// List returns a cursor over the tasks in the broker matching the given filter.
// Pass a nil filter to list all tasks. The caller must Close the returned cursor
// to release the underlying database transaction.
func (r *Radish) List(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
	return r.broker.List(ctx, filter)
}

func (r *Radish) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	return r.broker.Info(ctx, id)
}

func (r *Radish) Cancel(ctx context.Context, id int64) (err error) {
	return r.broker.Cancel(ctx, id)
}

func (r *Radish) Vacuum(ctx context.Context) (err error) {
	return r.broker.Vacuum(ctx, r.conf.Retention)
}

//============================================================================
// Executor
//============================================================================

func (e *executor) run(wg *sync.WaitGroup) {
	// Create an unbuffered channel to signal the poll loop to stop.
	// This channel should block until a signal is received.
	stop := make(chan struct{})

	// Execute the poll loop in a goroutine.
	go func(stop <-chan struct{}, wg *sync.WaitGroup) {
		defer wg.Done()
		poll := jitter.New(e.conf.PollInterval, e.conf.PollJitter)
		defer poll.Stop()

		// Poll for new tasks to execute
		for {
			select {
			case <-stop:
				return
			case <-poll.C:
				if err := e.dequeue(stop); err != nil {
					if errors.Is(err, ErrStop) {
						return
					}

					rlog.Fatal("fatal error while executing task", "error", err)
					return
				}
			}
		}
	}(stop, wg)

	// Store the stop channel for the executor to use to signal the poll loop to stop.
	e.stop = stop
}

func (e *executor) shutdown() {
	// Close the stop channel to signal the poll loop to stop and immediately return.
	// so that the executor is not blocking sending signals to the other executors.
	close(e.stop)
}

// Keep dequeuing tasks from the broker and executing them until there are no more
// tasks to dequeue or an error occurs (errors are considered fatal).
func (e *executor) dequeue(stop <-chan struct{}) (err error) {
	for {
		// Do not dequeue a task if the stop signal is received.
		select {
		case <-stop:
			return ErrStop
		default:
		}

		// Dequeue and execute the task.
		var task *models.TaskMeta
		if task, err = e.dequeueTask(); err != nil {
			if errors.Is(err, dberr.ErrNotFound) {
				return nil
			}
			return err
		}

		if err = e.execute(task); err != nil {
			return err
		}
	}
}

func (e *executor) dequeueTask() (task *models.TaskMeta, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.conf.PollInterval)
	defer cancel()

	return e.broker.Dequeue(ctx, e.conf.TaskTimeout)
}

func (e *executor) execute(task *models.TaskMeta) (err error) {
	// Create a logger with the task attributes for logging.
	taskLog := rlog.With("task", task.ID, "kind", task.Kind, "attempts", task.Attempts)

	// Find a worker that can execute the task.
	var worker internal.Worker
	if worker, err = e.workers.Get(task); err != nil {
		// If the task is not registered, log it
		// NOTE: not a fatal error so no error is returned.
		taskLog.Error("dequeued unregistered task", "error", err)
		return nil
	}

	// Get the task timeout from the worker.
	timeout := worker.Timeout()
	if timeout == 0 || timeout > e.conf.TaskTimeout {
		taskLog.Debug("using configured task timeout", "original_timeout", timeout)
		timeout = e.conf.TaskTimeout
	}

	// Execute the task with a timeout.
	taskLog.Debug("executing task", "timeout", timeout)
	taskctx, taskcancel := context.WithTimeout(context.Background(), timeout)
	defer taskcancel()

	// Unmarshal the task.
	if err = worker.UnmarshalTask(); err != nil {
		// If the task cannot be unmarshaled, log it and mark it as a failure.
		taskLog.Error("could not unmarshal task", "error", err)

		// Mark the task as failed.
		task.AddError(err, "")
		if err = e.broker.Fail(taskctx, task.ID, task.Errors); err != nil {
			// Inability to mark a task as failed is a fatal error.
			taskLog.Fatal("could not mark task as failed", "error", err)
			return err
		}
		return nil
	}

	if taskerr := e.recoveringDo(worker, taskctx); taskerr != nil {
		// Add the error to the task.
		AddError(task, taskerr)

		// If the task returns an error, check the retry policy and retry if necessary.
		var retry *internal.Retry
		if retry = worker.Retry(); retry == nil {
			retry = e.retryPolicy(task)
		}

		if retry.Retry {
			// Retry the task with the given delay.
			// If the delay is less than 0, use the default retry policy delay.
			delay := retry.Delay
			if delay < 0 {
				delay = e.retryPolicy(task).Delay
			}

			// Update the broker with the retry information.
			if err = e.broker.Retry(taskctx, task.ID, task.Errors, delay); err != nil {
				// Inability to retry the task is a fatal error.
				taskLog.Fatal("could not retry task", "error", err)
				return err
			}

			taskLog.Info("task failed, retrying", "delay", delay)
			return nil
		} else {
			// Mark the task as failed.
			if err = e.broker.Fail(taskctx, task.ID, task.Errors); err != nil {
				// Inability to mark a task as failed is a fatal error.
				taskLog.Fatal("could not mark task as failed", "error", err)
				return err
			}

			taskLog.Info(
				"task failed, no more retries",
				"duration", time.Since(task.LastAttempt.Time),
				"elapsed", time.Since(task.Created),
			)
			return nil
		}

	} else {
		// Mark the task as successful.
		if err = e.broker.Success(taskctx, task.ID); err != nil {
			// Inability to mark a task as successful is a fatal error.
			taskLog.Fatal("could not mark task as successful", "error", err)
			return err
		}

		taskLog.Info(
			"task completed successfully",
			"duration", time.Since(task.LastAttempt.Time),
			"elapsed", time.Since(task.Created),
		)
		return nil
	}
}

func (e *executor) recoveringDo(worker internal.Worker, ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Recover(r)
		}
	}()
	return worker.Do(ctx)
}

// Returns the default retry policy for the task.
func (e *executor) retryPolicy(task *models.TaskMeta) *internal.Retry {
	// Retry only if the number of attempts is less than the maximum number of allowed retries.
	return &internal.Retry{
		Retry: e.conf.TaskRetries > int(task.Attempts),
		Delay: e.backoff.Delay(int(task.Attempts)),
	}
}
