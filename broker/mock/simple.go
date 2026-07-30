package mock

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
)

type Simple struct {
	mu     sync.Mutex
	closed bool
	tasks  []*models.TaskMeta
}

func (s *Simple) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = false
	s.tasks = nil
}

func (s *Simple) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}

func (s *Simple) List(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, errors.ErrNotConnected
	}

	return nil, nil
}

func (s *Simple) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, errors.ErrNotConnected
	}

	if id < 1 {
		return nil, errors.ErrNotFound
	}

	if id > int64(len(s.tasks)) {
		return nil, errors.ErrNotFound
	}

	return s.tasks[id-1], nil
}

func (s *Simple) Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, errors.ErrNotConnected
	}

	if s.tasks == nil {
		s.tasks = make([]*models.TaskMeta, 0)
	}

	id = int64(len(s.tasks) + 1)
	task := &models.TaskMeta{
		ID:       id,
		Kind:     kind,
		Status:   status.Pending,
		Payload:  payload,
		Attempts: 0,
		Errors:   models.AttemptErrors{},
		Created:  time.Now(),
		Modified: time.Now(),
	}

	s.tasks = append(s.tasks, task)
	return id, nil
}

func (s *Simple) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, errors.ErrNotConnected
	}

	if s.tasks == nil {
		s.tasks = make([]*models.TaskMeta, 0)
	}

	id = int64(len(s.tasks) + 1)
	task := &models.TaskMeta{
		ID:        id,
		Kind:      kind,
		Status:    status.Scheduled,
		Payload:   payload,
		Attempts:  0,
		Errors:    models.AttemptErrors{},
		VisibleAt: sql.NullTime{Time: executeAfter, Valid: true},
		Created:   time.Now(),
		Modified:  time.Now(),
	}

	s.tasks = append(s.tasks, task)
	return id, nil
}

func (s *Simple) Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, errors.ErrNotConnected
	}

	if len(s.tasks) == 0 {
		// TODO: what does postgres return when there are no tasks found?
		return nil, errors.ErrNotFound
	}

	// Search for the next task that is pending or now visible.
	now := time.Now()
	for _, task := range s.tasks {
		switch task.Status {
		case status.Pending:
			task.Status = status.Running
			task.Attempts++
			task.VisibleAt = sql.NullTime{Time: now.Add(ttl), Valid: true}
			task.LastAttempt = sql.NullTime{Time: now, Valid: true}
			task.Modified = now
			return task, nil
		case status.Scheduled, status.Running, status.Retry:
			if task.VisibleAt.Time.Before(now) {
				task.Status = status.Running
				task.Attempts++
				task.VisibleAt = sql.NullTime{Time: now.Add(ttl), Valid: true}
				task.LastAttempt = sql.NullTime{Time: now, Valid: true}
				task.Modified = now
				return task, nil
			}
			continue
		default:
			continue
		}
	}

	// TODO: see above, return the error postgres returns when there are no tasks available.
	return nil, errors.ErrNotFound
}

func (s *Simple) Cancel(ctx context.Context, id int64) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.ErrNotConnected
	}

	if len(s.tasks) == 0 {
		// TODO: what does postgres return when there are no tasks found?
		return errors.ErrNotFound
	}

	if id < 1 {
		return errors.ErrNotFound
	}

	if id > int64(len(s.tasks)) {
		return errors.ErrNotFound
	}

	task := s.tasks[id-1]
	if task.Status != status.Pending && task.Status != status.Scheduled && task.Status != status.Retry {
		return errors.ErrTaskNotCancelable
	}

	task.Status = status.Cancelled
	task.VisibleAt = sql.NullTime{Valid: false}
	task.Finished = sql.NullTime{Time: time.Now(), Valid: true}
	task.Modified = time.Now()
	return nil
}

func (s *Simple) Fail(ctx context.Context, id int64, errs models.AttemptErrors) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.ErrNotConnected
	}

	if len(s.tasks) == 0 {
		// TODO: what does postgres return when there are no tasks found?
		return errors.ErrNotFound
	}

	if id < 1 {
		return errors.ErrNotFound
	}

	if id > int64(len(s.tasks)) {
		return errors.ErrNotFound
	}

	task := s.tasks[id-1]

	task.Status = status.Failed
	task.Errors = errs
	task.VisibleAt = sql.NullTime{Valid: false}
	task.Finished = sql.NullTime{Time: time.Now(), Valid: true}
	task.Modified = time.Now()
	return nil
}

func (s *Simple) Retry(ctx context.Context, id int64, errs models.AttemptErrors, delay time.Duration) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.ErrNotConnected
	}

	if len(s.tasks) == 0 {
		// TODO: what does postgres return when there are no tasks found?
		return errors.ErrNotFound
	}

	if id < 1 {
		return errors.ErrNotFound
	}

	if id > int64(len(s.tasks)) {
		return errors.ErrNotFound
	}

	task := s.tasks[id-1]
	task.Status = status.Retry
	task.Errors = errs
	task.VisibleAt = sql.NullTime{Time: time.Now().Add(delay), Valid: true}
	task.Modified = time.Now()
	return nil
}

func (s *Simple) Success(ctx context.Context, id int64) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.ErrNotConnected
	}

	if len(s.tasks) == 0 {
		// TODO: what does postgres return when there are no tasks found?
		return errors.ErrNotFound
	}

	if id < 1 {
		return errors.ErrNotFound
	}

	if id > int64(len(s.tasks)) {
		return errors.ErrNotFound
	}

	task := s.tasks[id-1]
	task.Status = status.Succeeded
	task.VisibleAt = sql.NullTime{Valid: false}
	task.Finished = sql.NullTime{Time: time.Now(), Valid: true}
	task.Modified = time.Now()
	return nil
}

func (s *Simple) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.ErrNotConnected
	}

	ts := time.Now().Add(-retention)
	for i, task := range s.tasks {
		if task.Status == status.Succeeded || task.Status == status.Failed || task.Status == status.Cancelled {
			if task.Finished.Time.Before(ts) {
				s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			}
		}
	}

	return nil
}
