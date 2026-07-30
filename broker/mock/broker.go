package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/dsn"
)

const (
	Close    = "Close"
	List     = "List"
	Info     = "Info"
	Enqueue  = "Enqueue"
	Schedule = "Schedule"
	Dequeue  = "Dequeue"
	Cancel   = "Cancel"
	Fail     = "Fail"
	Retry    = "Retry"
	Success  = "Success"
	Vacuum   = "Vacuum"
)

var ErrNoMockFunction = errors.New("no mock function provided")

func ErrNoMock(name string) error {
	return fmt.Errorf("no mock function provided for %s: %w", name, ErrNoMockFunction)
}

type Broker struct {
	OnClose    func() error
	OnList     func(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error)
	OnInfo     func(ctx context.Context, id int64) (task *models.TaskMeta, err error)
	OnEnqueue  func(ctx context.Context, kind string, payload []byte) (id int64, err error)
	OnSchedule func(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error)
	OnDequeue  func(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error)
	OnCancel   func(ctx context.Context, id int64) (err error)
	OnFail     func(ctx context.Context, id int64, errors models.AttemptErrors) (err error)
	OnRetry    func(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error)
	OnSuccess  func(ctx context.Context, id int64) (err error)
	OnVacuum   func(ctx context.Context, retention time.Duration) (err error)

	mu    sync.Mutex
	calls map[string]int
}

func Connect(uri *dsn.DSN) (b *Broker, err error) {
	return &Broker{}, nil
}

func (b *Broker) Reset() {
	b.mu.Lock()
	b.OnClose = nil
	b.OnList = nil
	b.OnInfo = nil
	b.OnEnqueue = nil
	b.OnSchedule = nil
	b.OnDequeue = nil
	b.OnCancel = nil
	b.OnFail = nil
	b.OnRetry = nil
	b.OnSuccess = nil
	b.OnVacuum = nil
	b.calls = nil
	b.mu.Unlock()
}

func (b *Broker) ErrorOn(name string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch name {
	case Close:
		b.OnClose = func() error {
			return err
		}
	case List:
		b.OnList = func(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
			return nil, err
		}
	case Info:
		b.OnInfo = func(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
			return nil, err
		}
	case Enqueue:
		b.OnEnqueue = func(ctx context.Context, kind string, payload []byte) (id int64, err error) {
			return 0, err
		}
	case Schedule:
		b.OnSchedule = func(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
			return 0, err
		}
	case Dequeue:
		b.OnDequeue = func(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
			return nil, err
		}
	case Cancel:
		b.OnCancel = func(ctx context.Context, id int64) (err error) {
			return err
		}
	case Fail:
		b.OnFail = func(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
			return err
		}
	case Retry:
		b.OnRetry = func(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
			return err
		}
	case Success:
		b.OnSuccess = func(ctx context.Context, id int64) (err error) {
			return err
		}
	case Vacuum:
		b.OnVacuum = func(ctx context.Context, retention time.Duration) (err error) {
			return err
		}
	default:
		panic(fmt.Sprintf("unknown broker method: %s", name))
	}
}

func (b *Broker) AssertNCalls(t *testing.T, name string, n int) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.calls == nil {
		t.Fatalf("no calls were made to broker.%s()", name)
	}

	if count, ok := b.calls[name]; ok && count != n {
		t.Fatalf("broker.%s() was called %d times, expected %d", name, count, n)
	}
}

func (b *Broker) AssertCalled(t *testing.T, name string) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.calls == nil {
		t.Fatalf("no calls were made to broker.%s()", name)
	}

	if count, ok := b.calls[name]; !ok || count == 0 {
		t.Fatalf("broker.%s() was not called", name)
	}
}

func (b *Broker) AssertNotCalled(t *testing.T, name string) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.calls == nil {
		return
	}

	if count, ok := b.calls[name]; ok && count > 0 {
		t.Fatalf("broker.%s() was called %d times", name, count)
	}
}

func (b *Broker) Close() error {
	b.incr(Close)
	if b.OnClose != nil {
		return b.OnClose()
	}
	return ErrNoMock(Close)
}

func (b *Broker) List(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
	b.incr(List)
	if b.OnList != nil {
		return b.OnList(ctx, filter)
	}
	return nil, ErrNoMock(List)
}

func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	b.incr(Info)
	if b.OnInfo != nil {
		return b.OnInfo(ctx, id)
	}
	return nil, ErrNoMock(Info)
}

func (b *Broker) Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	b.incr(Enqueue)
	if b.OnEnqueue != nil {
		return b.OnEnqueue(ctx, kind, payload)
	}
	return 0, ErrNoMock(Enqueue)
}

func (b *Broker) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	b.incr(Schedule)
	if b.OnSchedule != nil {
		return b.OnSchedule(ctx, kind, payload, executeAfter)
	}
	return 0, ErrNoMock(Schedule)
}

func (b *Broker) Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	b.incr(Dequeue)
	if b.OnDequeue != nil {
		return b.OnDequeue(ctx, ttl)
	}
	return nil, ErrNoMock(Dequeue)
}

func (b *Broker) Cancel(ctx context.Context, id int64) (err error) {
	b.incr(Cancel)
	if b.OnCancel != nil {
		return b.OnCancel(ctx, id)
	}
	return ErrNoMock(Cancel)
}

func (b *Broker) Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
	b.incr(Fail)
	if b.OnFail != nil {
		return b.OnFail(ctx, id, errors)
	}
	return ErrNoMock(Fail)
}

func (b *Broker) Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
	b.incr(Retry)
	if b.OnRetry != nil {
		return b.OnRetry(ctx, id, errors, delay)
	}
	return ErrNoMock(Retry)
}

func (b *Broker) Success(ctx context.Context, id int64) (err error) {
	b.incr(Success)
	if b.OnSuccess != nil {
		return b.OnSuccess(ctx, id)
	}
	return ErrNoMock(Success)
}

func (b *Broker) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	b.incr(Vacuum)
	if b.OnVacuum != nil {
		return b.OnVacuum(ctx, retention)
	}
	return ErrNoMock(Vacuum)
}

func (b *Broker) incr(name string) {
	b.mu.Lock()
	if b.calls == nil {
		b.calls = make(map[string]int)
	}
	b.calls[name]++
	b.mu.Unlock()
}
