package radish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.rtnl.ai/radish/backoff"
	"go.rtnl.ai/radish/broker/mock"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/rlog"
)

func TestRadish(t *testing.T) {
	conf := mockConfig(t)

	turnip, err := New(conf)
	require.NoError(t, err)

	require.NoError(t, Register(turnip, new(SleepWorker)))
	require.NoError(t, Register(turnip, new(SortWorker)))
	require.NoError(t, Register(turnip, new(RandomFailureWorker)))
}

func TestGracefulShutdown(t *testing.T) {
	t.Skip("not implemented yet")
	// TODO: test that radish waits for all executors to finish before returning from shutdown.
}

// TestRuntimeErrorHandler verifies that dequeue errors are reported to the
// configured handler without terminating the executor or the process.
func TestRuntimeErrorHandler(t *testing.T) {
	expected := errors.New("dequeue failed")
	var (
		receivedCtx    context.Context
		receivedCtxErr error
		receivedTask   *models.TaskMeta
		receivedErr    error
	)
	conf := Config{
		BookkeepingTimeout: 100 * time.Millisecond,
		OnError: func(ctx context.Context, task *models.TaskMeta, err error) {
			receivedCtx = ctx
			receivedCtxErr = ctx.Err()
			receivedTask = task
			receivedErr = err
		},
	}
	broker := &mock.Broker{
		OnDequeue: func(context.Context, time.Duration) (*models.TaskMeta, error) {
			return nil, expected
		},
	}
	meter, err := NewMetrics(nil)
	require.NoError(t, err)

	executor := &executor{
		conf:    &conf,
		workers: &Workers{},
		broker:  broker,
		tracer:  noop.NewTracerProvider().Tracer("test"),
		meter:   meter,
	}
	more, err := executor.dequeueOne()
	require.NoError(t, err)
	require.False(t, more)
	require.ErrorIs(t, receivedErr, expected)
	require.Nil(t, receivedTask)
	require.NotNil(t, receivedCtx)
	require.NoError(t, receivedCtxErr)
}

// TestRuntimeErrorHandlerReceivesTask verifies that finalization errors pass
// the affected task to the application handler.
func TestRuntimeErrorHandlerReceivesTask(t *testing.T) {
	expected := errors.New("success failed")
	task := &models.TaskMeta{
		ID:      1,
		Kind:    new(bookkeepingTask).Kind(),
		Payload: []byte(`{}`),
	}
	var receivedTask *models.TaskMeta
	conf := Config{
		TaskTimeout:        time.Second,
		BookkeepingTimeout: 100 * time.Millisecond,
		OnError: func(_ context.Context, task *models.TaskMeta, _ error) {
			receivedTask = task
		},
	}
	broker := &mock.Broker{
		OnDequeue: func(context.Context, time.Duration) (*models.TaskMeta, error) {
			return task, nil
		},
		OnSuccess: func(context.Context, int64) error {
			return expected
		},
	}
	meter, err := NewMetrics(nil)
	require.NoError(t, err)

	executor := &executor{
		conf:    &conf,
		workers: &Workers{},
		broker:  broker,
		tracer:  noop.NewTracerProvider().Tracer("test"),
		meter:   meter,
	}
	require.NoError(t, AddWorkerSafe(executor.workers, &bookkeepingWorker{mode: "success"}))

	more, err := executor.dequeueOne()
	require.NoError(t, err)
	require.False(t, more)
	require.Same(t, task, receivedTask)
}

// TestDefaultErrorHandlerFatal verifies that runtime errors use rlog.Fatal
// when no application error handler is configured.
func TestDefaultErrorHandlerFatal(t *testing.T) {
	fatalCalled := false
	rlog.SetFatalHook(func() {
		fatalCalled = true
	})
	t.Cleanup(func() {
		rlog.SetFatalHook(nil)
	})

	conf := Config{BookkeepingTimeout: 100 * time.Millisecond}
	executor := &executor{conf: &conf}
	executor.handleError(context.Background(), nil, errors.New("runtime failure"))

	require.True(t, fatalCalled)
}

//=====================================
// Bookkeeping Tests
//=====================================

// TestBookkeepingAfterTaskTimeout verifies that success, retry, and terminal
// failure can be finalized after the worker task context expires.
func TestBookkeepingAfterTaskTimeout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		assert func(*mock.Broker)
	}{
		{
			name: "success",
			mode: "success",
			assert: func(b *mock.Broker) {
				b.OnSuccess = bookkeepingOperation
			},
		},
		{
			name: "retry",
			mode: "retry",
			assert: func(b *mock.Broker) {
				b.OnRetry = func(ctx context.Context, _ int64, _ models.AttemptErrors, _ time.Duration) error {
					return bookkeepingOperation(ctx, 0)
				}
			},
		},
		{
			name: "failure",
			mode: "failure",
			assert: func(b *mock.Broker) {
				b.OnFail = func(ctx context.Context, _ int64, _ models.AttemptErrors) error {
					return bookkeepingOperation(ctx, 0)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := Config{
				TaskRetries:        1,
				TaskTimeout:        time.Second,
				BookkeepingTimeout: 100 * time.Millisecond,
				Backoff:            backoff.Config{Policy: backoff.PolicyZero},
			}
			broker := &mock.Broker{}
			tc.assert(broker)

			meter, err := NewMetrics(nil)
			require.NoError(t, err)

			executor := &executor{
				conf:    &conf,
				workers: &Workers{},
				broker:  broker,
				backoff: &backoff.ZeroBackOff{},
				tracer:  noop.NewTracerProvider().Tracer("test"),
				meter:   meter,
			}
			require.NoError(t, AddWorkerSafe(executor.workers, &bookkeepingWorker{mode: tc.mode}))

			task := &models.TaskMeta{
				ID:      1,
				Kind:    new(bookkeepingTask).Kind(),
				Payload: []byte(`{}`),
			}
			require.NoError(t, executor.execute(context.Background(), task))
		})
	}
}

// TestBookkeepingAfterUnmarshalFailure verifies that invalid task payloads can
// still be marked failed through the bookkeeping context.
func TestBookkeepingAfterUnmarshalFailure(t *testing.T) {
	conf := Config{
		TaskTimeout:        time.Second,
		BookkeepingTimeout: 100 * time.Millisecond,
	}
	broker := &mock.Broker{
		OnFail: func(ctx context.Context, _ int64, _ models.AttemptErrors) error {
			return bookkeepingOperation(ctx, 0)
		},
	}
	meter, err := NewMetrics(nil)
	require.NoError(t, err)

	executor := &executor{
		conf:    &conf,
		workers: &Workers{},
		broker:  broker,
		tracer:  noop.NewTracerProvider().Tracer("test"),
		meter:   meter,
	}
	require.NoError(t, AddWorkerSafe(executor.workers, &bookkeepingWorker{}))

	task := &models.TaskMeta{
		ID:      1,
		Kind:    new(bookkeepingTask).Kind(),
		Payload: []byte(`{"invalid"`),
	}
	require.NoError(t, executor.execute(context.Background(), task))
}

// TestWorkerTimeoutReceivesTask verifies that custom timeout handlers receive
// the unmarshaled task before the worker timeout is evaluated and started.
func TestWorkerTimeoutReceivesTask(t *testing.T) {
	conf := Config{
		TaskTimeout:        time.Second,
		BookkeepingTimeout: 100 * time.Millisecond,
	}
	broker := &mock.Broker{
		OnSuccess: func(context.Context, int64) error {
			return nil
		},
	}
	meter, err := NewMetrics(nil)
	require.NoError(t, err)

	worker := &bookkeepingWorker{mode: "success"}
	executor := &executor{
		conf:    &conf,
		workers: &Workers{},
		broker:  broker,
		tracer:  noop.NewTracerProvider().Tracer("test"),
		meter:   meter,
	}
	require.NoError(t, AddWorkerSafe(executor.workers, worker))

	task := &models.TaskMeta{
		ID:      1,
		Kind:    new(bookkeepingTask).Kind(),
		Payload: []byte(`{}`),
	}
	require.NoError(t, executor.execute(context.Background(), task))
	require.True(t, worker.timeoutSawTask)
}

func bookkeepingOperation(ctx context.Context, _ int64) error {
	select {
	case <-time.After(10 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type bookkeepingTask struct{}

func (*bookkeepingTask) Kind() string {
	return "bookkeeping"
}

type bookkeepingWorker struct {
	mode           string
	timeoutSawTask bool
}

func (w *bookkeepingWorker) Retry(*TaskInfo[*bookkeepingTask]) *Retry {
	return &Retry{Retry: w.mode == "retry"}
}

func (w *bookkeepingWorker) Timeout(info *TaskInfo[*bookkeepingTask]) time.Duration {
	w.timeoutSawTask = info != nil && info.Task != nil
	return time.Millisecond
}

func (w *bookkeepingWorker) Do(ctx context.Context, _ *TaskInfo[*bookkeepingTask]) error {
	<-ctx.Done()
	if w.mode == "success" {
		return nil
	}
	return errors.New("worker failed")
}
