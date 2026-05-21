package postgres

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/models"
)

type Broker struct {
	mu sync.RWMutex
	db *sql.DB
}

func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	return nil, nil
}

const enqueueSQL = `
-- Enqueue a task with the given kind and payload.
-- Parameter: the kind of task to enqueue and its JSON encoded payload.
INSERT INTO radish_tasks (kind, payload) VALUES ($1, $2) RETURNING id;
`

// Enqueue a task with the given kind and payload.
func (b *Broker) Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	var row *sql.Row
	if row, err = b.QueryRow(ctx, enqueueSQL, kind, payload); err != nil {
		return 0, err
	}

	if err = row.Scan(&id); err != nil {
		return 0, dbe(err)
	}

	if err = row.Err(); err != nil {
		return 0, dbe(err)
	}

	return id, nil
}

const scheduleSQL = `
-- Schedule a task with the given kind and payload to be executed later.
-- Parameter: the kind of task to schedule and its JSON encoded payload and the timestamp when it should be executed after.
INSERT INTO radish_tasks (kind, status, payload, visible_at)
    VALUES ($1, 'scheduled', $2, $3) RETURNING id;
`

// Schedule a task with the given kind and payload to be executed later.
func (b *Broker) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	var row *sql.Row
	if row, err = b.QueryRow(ctx, scheduleSQL, kind, payload, executeAfter); err != nil {
		return 0, err
	}

	if err = row.Scan(&id); err != nil {
		return 0, dbe(err)
	}

	if err = row.Err(); err != nil {
		return 0, dbe(err)
	}

	return id, nil
}

const dequeueSQL = `
-- Dequeue the next task from the queue, skipping over locked rows.
-- Parameter: the visibility timeout for the task in seconds.
WITH next_task AS (
    SELECT id FROM radish_tasks
    WHERE
        status = 'pending' OR
        (status in ('running', 'scheduled', 'retry') AND visible_at <= NOW())
    ORDER BY created ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE radish_tasks
SET status = 'running',
    attempts = attempts + 1,
    visible_at = NOW() + make_interval(secs := $1),
    last_attempt = NOW(),
    modified = NOW()
FROM next_task
WHERE radish_tasks.id = next_task.id
RETURNING radish_tasks.*;
`

// Dequeue the next task from the queue, skipping over locked rows.
// The TTL is the time that the dequeueing entity has to complete the task before it
// may be considered failed and re-assigned to a different worker.
func (b *Broker) Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	var row *sql.Row
	if row, err = b.QueryRow(ctx, dequeueSQL, ttl.Seconds()); err != nil {
		return nil, err
	}

	task = &models.TaskMeta{}
	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const cancelSQL = `
-- Mark a task as cancelled with no additional errors.
UPDATE radish_tasks
SET status = 'cancelled',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1 AND status in ('pending', 'scheduled', 'retry');
`

// Mark a task as cancelled with no additional errors.
// NOTE: if the task is already in progress or has been completed an error will be
// returned that the task cannot be cancelled.
func (b *Broker) Cancel(ctx context.Context, id int64) (err error) {
	var result sql.Result
	if result, err = b.Exec(ctx, cancelSQL, id); err != nil {
		return err
	}

	var rows int64
	if rows, err = result.RowsAffected(); err != nil {
		return dbe(err)
	}

	if rows == 0 {
		return errors.ErrTaskNotCancelable
	}
	return nil
}

const failedSQL = `
-- Mark a task as failed with the given errors.
UPDATE radish_tasks
SET status = 'failed',
    errors = $2,
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1;
`

// Mark a task as failed with the given errors.
func (b *Broker) Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
	if _, err = b.Exec(ctx, failedSQL, id, errors); err != nil {
		return err
	}
	return nil
}

const retrySQL = `
-- Mark a task as pending with the given errors and retry the task.
-- Parameters: the task id, the errors to add to the task, and the delay before the next retry in seconds.
UPDATE radish_tasks
SET status = 'retry',
    errors = $2,
    visible_at = NOW() + make_interval(secs := $3),
    modified = NOW()
WHERE id = $1;
`

// Mark a task as retryable with the given errors and backoff delay.
func (b *Broker) Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
	if _, err = b.Exec(ctx, retrySQL, id, errors, delay.Seconds()); err != nil {
		return err
	}
	return nil
}

const successSQL = `
-- Mark a task as succeeded with no additional errors.
UPDATE radish_tasks
SET status = 'succeeded',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1;
`

// Mark a task as succeeded with no additional errors.
func (b *Broker) Success(ctx context.Context, id int64) (err error) {
	if _, err = b.Exec(ctx, successSQL, id); err != nil {
		return err
	}
	return nil
}

const vacuumSQL = `
-- Cleanup any completed tasks that are older than the retention period.
-- Parameter: the retention period in seconds.
DELETE FROM radish_tasks WHERE
    status IN ('succeeded', 'failed', 'cancelled') AND
    finished < NOW() - make_interval(secs := $1);
`

// Cleans up any completed tasks that are older than the retention period.
func (b *Broker) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	if _, err = b.Exec(ctx, vacuumSQL, retention.Seconds()); err != nil {
		return err
	}

	return nil
}
