package radish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/radish/backoff"
	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/radish/broker/cursor"
	dberr "go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/broker/options"
	"go.rtnl.ai/radish/broker/postgres"
	internal "go.rtnl.ai/radish/internal/worker"
	"go.rtnl.ai/radish/jitter"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
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
	tracer    trace.Tracer
	meter     *Metrics
}

type executor struct {
	conf    *Config
	workers *Workers
	broker  broker.Broker
	backoff backoff.BackOff
	stop    chan<- struct{}
	tracer  trace.Tracer
	meter   *Metrics
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

	// Create metrics from the metric provider.
	var meter *Metrics
	if meter, err = NewMetrics(conf.MetricProvider); err != nil {
		return nil, err
	}

	return &Radish{
		conf:      *conf,
		executors: nil,
		workers: &Workers{
			workers: make(map[string]untypedWorker),
		},
		tracer: conf.NewTracer(),
		meter:  meter,
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

	// Register the queue size callback after connecting to the database.
	if err = r.meter.RegisterQueueSizeCallback(r.QueueSize); err != nil {
		return err
	}

	// Create a wait group to wait for all executors to finish.
	r.wg = &sync.WaitGroup{}
	r.wg.Add(r.conf.NumWorkers)

	// Create the executors with a copy of the config and workers to execute tasks in parallel.
	for i := 0; i < r.conf.NumWorkers; i++ {
		executor := &executor{
			conf:    &r.conf,
			workers: r.workers,
			broker:  r.broker,
			backoff: policy,
			tracer:  r.tracer,
			meter:   r.meter,
		}
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

	// Unregister the telemetry callback.
	if err := r.meter.Shutdown(); err != nil {
		rlog.Error("could not shutdown telemetry", "error", err)
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
	ctx, span := r.tracer.Start(ctx, "radish.Enqueue")
	defer span.End()

	var data []byte
	if data, err = json.Marshal(task); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not marshal task json")
		return 0, fmt.Errorf("could not marshal task json: %w", err)
	}

	var brokerOptions *options.Options
	if len(opts) > 0 {
		brokerOptions = &options.Options{}
		for _, opt := range opts {
			opt(brokerOptions)
		}

		// Add the task kinds to the broker options
		brokerOptions.Kinds = []string{task.Kind()}
		if taskWithAliases, ok := task.(TaskWithAliases); ok {
			brokerOptions.Kinds = append(brokerOptions.Kinds, taskWithAliases.KindAliases()...)
		}
	}

	id, err = r.broker.Enqueue(ctx, task.Kind(), data, brokerOptions)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "enqueue operation failed")

	} else {
		r.meter.incrSentMessages(ctx, task.Kind())
	}

	return id, err
}

func (r *Radish) Schedule(ctx context.Context, task Task, executeAfter time.Time, opts ...Option) (id int64, err error) {
	ctx, span := r.tracer.Start(ctx, "radish.Schedule")
	defer span.End()

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

		// Add the task kinds to the broker options
		brokerOptions.Kinds = []string{task.Kind()}
		if taskWithAliases, ok := task.(TaskWithAliases); ok {
			brokerOptions.Kinds = append(brokerOptions.Kinds, taskWithAliases.KindAliases()...)
		}
	}

	id, err = r.broker.Schedule(ctx, task.Kind(), data, executeAfter, brokerOptions)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "schedule operation failed")
	} else {
		r.meter.incrSentMessages(ctx, task.Kind())
	}

	return id, err
}

// List returns a cursor over the tasks in the broker matching the given filter.
// Pass a nil filter to list all tasks. The caller must Close the returned cursor
// to release the underlying database transaction.
func (r *Radish) List(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
	ctx, span := r.tracer.Start(ctx, "radish.List")
	defer span.End()

	tasks, err = r.broker.List(ctx, filter)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list operation failed")
	}

	return tasks, err
}

func (r *Radish) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	ctx, span := r.tracer.Start(ctx, "radish.Info")
	defer span.End()

	task, err = r.broker.Info(ctx, id)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "info operation failed")
	}

	return task, err
}

func (r *Radish) Cancel(ctx context.Context, id int64) (err error) {
	ctx, span := r.tracer.Start(ctx, "radish.Cancel")
	defer span.End()

	err = r.broker.Cancel(ctx, id)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "cancel operation failed")
	} else {
		// Update the metrics with the task kind that was cancelled.
		if info, err := r.broker.Info(ctx, id); err == nil {
			r.meter.consumeCancelledMessages(ctx, info.Kind)
			r.meter.recordCompletedTask(ctx, info.Kind, status.Cancelled)
		} else {
			r.meter.consumeCancelledMessages(ctx, "unknown")
			r.meter.recordCompletedTask(ctx, "unknown", status.Cancelled)
		}
	}

	return err
}

func (r *Radish) Vacuum(ctx context.Context) (err error) {
	ctx, span := r.tracer.Start(ctx, "radish.Vacuum")
	defer span.End()

	err = r.broker.Vacuum(ctx, r.conf.Retention)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "vacuum operation failed")
	}

	return err
}

func (r *Radish) QueueStatus(ctx context.Context) (out *models.QueueStatus, err error) {
	ctx, span := r.tracer.Start(ctx, "radish.QueueStatus")
	defer span.End()

	out, err = r.broker.QueueStatus(ctx)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "queue status operation failed")
	}

	return out, err
}

func (r *Radish) TimeSeries(ctx context.Context, after, before time.Time, interval time.Duration) (series models.Series, err error) {
	ctx, span := r.tracer.Start(ctx, "radish.TimeSeries")
	defer span.End()

	series, err = r.broker.TimeSeries(ctx, after, before, interval)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "time series operation failed")
	}

	return series, err
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

					e.handleError(context.Background(), nil, err)
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
// tasks to dequeue or an error occurs.
func (e *executor) dequeue(stop <-chan struct{}) (err error) {
	for {
		// Do not dequeue a task if the stop signal is received.
		select {
		case <-stop:
			return ErrStop
		default:
		}

		var more bool
		if more, err = e.dequeueOne(); err != nil {
			return err
		}

		// Stop if the there are no more tasks to dequeue and wait the poll interval
		// before trying to dequeue again. If there are more tasks to dequeue, continue.
		if !more {
			return nil
		}
	}
}

func (e *executor) dequeueOne() (more bool, err error) {
	// Create a context and a span for each dequeue operation.
	ctx, span := e.tracer.Start(context.Background(), "radish.Dequeue")
	defer span.End()

	// Dequeue and execute the task.
	var task *models.TaskMeta
	if task, err = e.dequeueTask(ctx); err != nil {
		if errors.Is(err, dberr.ErrNotFound) {
			return false, nil
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "unable to dequeue task")
		e.handleError(ctx, nil, err)
		return false, nil
	}

	if err = e.execute(ctx, task); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "unable to execute task")
		e.handleError(ctx, task, err)
		return false, nil
	}

	// Successfully executed the task, continue dequeuing.
	return true, nil
}

func (e *executor) dequeueTask(ctx context.Context) (task *models.TaskMeta, err error) {
	ctx, cancel := context.WithTimeout(ctx, e.conf.PollInterval)
	defer cancel()

	if task, err = e.broker.Dequeue(ctx, e.conf.TaskTimeout); err == nil {
		e.meter.incrConsumedMessages(ctx, task.Kind)
	}
	return task, err
}

func (e *executor) execute(ctx context.Context, task *models.TaskMeta) (err error) {
	start := time.Now()
	ctx, span := e.tracer.Start(ctx, "radish.Execute")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "unable to execute task")
		}
		span.End()
	}()

	// Create a logger with the task attributes for logging.
	taskLog := rlog.With("task", task.ID, "kind", task.Kind, "attempts", task.Attempts)

	// Find a worker that can execute the task.
	var worker internal.Worker
	if worker, err = e.workers.Get(task); err != nil {
		// If the task is not registered, log it
		// NOTE: not a fatal error so no error is returned.
		e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "unregistered_task")
		taskLog.Error("dequeued unregistered task", "error", err)

		span.RecordError(err)
		span.SetStatus(codes.Error, "unregistered task")
		return nil
	}

	// Unmarshal the task.
	if err = worker.UnmarshalTask(); err != nil {
		// If the task cannot be unmarshaled, log it and mark it as a failure.
		taskLog.Error("could not unmarshal task", "error", err)

		// Use a separate context for bookkeeping.
		bookkeepingctx, bookkeepingcancel := e.bookkeepingContext(ctx)
		defer bookkeepingcancel()

		// Mark the task as failed.
		task.AddError(err, "")
		if err = e.broker.Fail(bookkeepingctx, task.ID, task.Errors); err != nil {
			e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "broker:critical")
			taskLog.Error("could not mark task as failed", "error", err)
			return err
		}
		e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "json:unmarshal_error")
		return nil
	}

	// Get the task timeout from the worker after unmarshaling so custom timeout
	// handlers can inspect the task payload. The maximum allowed timeout is the
	// configured task timeout, so if the worker's timeout is longer then the
	// default is applied here.
	timeout := worker.Timeout()
	if timeout == 0 || timeout > e.conf.TaskTimeout {
		taskLog.Debug("using configured task timeout", "original_timeout", timeout)
		timeout = e.conf.TaskTimeout
	}

	// Execute the task with a timeout.
	taskLog.Debug("executing task", "timeout", timeout)
	taskctx, taskcancel := context.WithTimeout(ctx, timeout)
	defer taskcancel()

	if taskerr := e.recoveringDo(worker, taskctx); taskerr != nil {
		// Use a separate context for bookkeeping.
		bookkeepingctx, bookkeepingcancel := e.bookkeepingContext(ctx)
		defer bookkeepingcancel()

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
			if err = e.broker.Retry(bookkeepingctx, task.ID, task.Errors, delay); err != nil {
				e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "broker:critical")
				taskLog.Error("could not retry task", "error", err)
				return err
			}

			e.meter.incrSentMessages(ctx, task.Kind)
			e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "radish:retry")
			taskLog.Info("task failed, retrying", "delay", delay)
			return nil
		} else {
			// Mark the task as failed.
			if err = e.broker.Fail(bookkeepingctx, task.ID, task.Errors); err != nil {
				e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "broker:critical")
				taskLog.Error("could not mark task as failed", "error", err)
				return err
			}

			taskLog.Info(
				"task failed, no more retries",
				"duration", time.Since(task.LastAttempt.Time),
				"elapsed", time.Since(task.Created),
			)
			e.meter.recordCompletedTask(ctx, task.Kind, status.Failed)
			e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "radish:failed")
			return nil
		}

	} else {
		// Use a separate context for bookkeeping.
		bookkeepingctx, bookkeepingcancel := e.bookkeepingContext(ctx)
		defer bookkeepingcancel()

		// Mark the task as successful.
		if err = e.broker.Success(bookkeepingctx, task.ID); err != nil {
			// Inability to mark a task as successful is a fatal error.
			e.meter.recordTaskDurationFailed(ctx, time.Since(start), task.Kind, "broker:critical")
			taskLog.Error("could not mark task as successful", "error", err)
			return err
		}

		taskLog.Info(
			"task completed successfully",
			"duration", time.Since(task.LastAttempt.Time),
			"elapsed", time.Since(task.Created),
		)
		e.meter.recordCompletedTask(ctx, task.Kind, status.Succeeded)
		e.meter.recordTaskDuration(ctx, time.Since(start), task.Kind)
		return nil
	}
}

func (e *executor) bookkeepingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), e.conf.BookkeepingTimeout)
}

// handleError reports a runtime executor error. Errors are fatal by default;
// configuring OnError overrides the default and delegates handling to the
// application with a detached bookkeeping context.
func (e *executor) handleError(ctx context.Context, task *models.TaskMeta, err error) {
	if e.conf.OnError == nil {
		rlog.Fatal("fatal error while executing task", "error", err)
		return
	}

	ctx, cancel := e.bookkeepingContext(ctx)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			rlog.Error("runtime error handler panicked", "error", Recover(r))
		}
	}()

	e.conf.OnError(ctx, task, err)
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
