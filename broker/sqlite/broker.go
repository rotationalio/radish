package sqlite

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
)

type Broker struct {
	mu sync.RWMutex
	db *sql.DB
}

const infoSQL = `
SELECT id, kind, status, json_extract(payload, '$') as payload, attempts, json_extract(errors, '$') as errors, visible_at, last_attempt, finished, created, modified
FROM radish_tasks WHERE id=:id;
`

// Get information about a task by its id.
func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	params := []any{
		sql.Named("id", id),
	}

	var row *sql.Row
	if row, err = b.QueryRow(ctx, infoSQL, params...); err != nil {
		return nil, err
	}

	task = &models.TaskMeta{}
	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const enqueueSQL = `
INSERT INTO radish_tasks (kind, payload) VALUES (:kind, jsonb(:payload)) RETURNING id;
`

// Enqueue a task with the given kind and payload.
func (b *Broker) Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	params := []any{
		sql.Named("kind", kind),
		sql.Named("payload", payload),
	}

	var row *sql.Row
	if row, err = b.QueryRow(ctx, enqueueSQL, params...); err != nil {
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
INSERT INTO radish_tasks (kind, status, payload, visible_at)
	VALUES (:kind, 'scheduled', jsonb(:payload), :visibleAt) RETURNING id;
`

// Schedule a task with the given kind and payload to be executed later.
func (b *Broker) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	params := []any{
		sql.Named("kind", kind),
		sql.Named("payload", payload),
		sql.Named("visibleAt", executeAfter),
	}

	var row *sql.Row
	if row, err = b.QueryRow(ctx, scheduleSQL, params...); err != nil {
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

const dequeueSelectSQL = `
SELECT id, kind, status, json_extract(payload, '$') as payload, attempts, json_extract(errors, '$') as errors, visible_at, last_attempt, finished, created, modified
FROM radish_tasks
WHERE
	status = 'pending' OR
	(status in ('running', 'scheduled', 'retry') AND visible_at <= datetime('now'))
ORDER BY created ASC
LIMIT 1
`

const dequeueUpdateSQL = `
UPDATE radish_tasks
SET status = :status,
	attempts = :attempts,
	visible_at = :visibleAt,
	last_attempt = :lastAttempt,
	modified = :modified
WHERE id=:id
`

// Dequeue the next task from the queue, skipping over locked rows.
// SQLite3 relies on the global write lock to ensure that only one task is worked at a time.
// NOTE: BEGIN IMMEDIATE is used in the transaction to ensure that the task is locked.
func (b *Broker) Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable}); err != nil {
		return nil, err
	}
	defer tx.Rollback()

	task = &models.TaskMeta{}
	if err = task.Scan(tx.QueryRow(dequeueSelectSQL)); err != nil {
		return nil, dbe(err)
	}

	task.Status = status.Running
	task.Attempts++

	now := time.Now()
	task.VisibleAt = sql.NullTime{Time: now.Add(ttl), Valid: true}
	task.LastAttempt = sql.NullTime{Time: now, Valid: true}
	task.Modified = now

	params := []any{
		sql.Named("id", task.ID),
		sql.Named("status", task.Status),
		sql.Named("attempts", task.Attempts),
		sql.Named("visibleAt", task.VisibleAt),
		sql.Named("lastAttempt", task.LastAttempt),
		sql.Named("modified", task.Modified),
	}
	if _, err = tx.Exec(dequeueUpdateSQL, params...); err != nil {
		return nil, dbe(err)
	}

	if err = tx.Commit(); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const cancelSQL = `
UPDATE radish_tasks
SET status = 'cancelled',
	visible_at = NULL,
	finished = datetime('now'),
	modified = datetime('now')
WHERE id=:id AND status in ('pending', 'scheduled', 'retry')
`

// Mark a task as cancelled with no additional errors.
func (b *Broker) Cancel(ctx context.Context, id int64) (err error) {
	params := []any{
		sql.Named("id", id),
	}

	var result sql.Result
	if result, err = b.Exec(ctx, cancelSQL, params...); err != nil {
		return dbe(err)
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

const failSQL = `
UPDATE radish_tasks
SET status = 'failed',
	errors = jsonb(:errors),
	visible_at = NULL,
	finished = datetime('now'),
	modified = datetime('now')
WHERE id=:id
`

// Mark a task as failed with the given errors.
func (b *Broker) Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
	params := []any{
		sql.Named("id", id),
		sql.Named("errors", errors),
	}

	if _, err = b.Exec(ctx, failSQL, params...); err != nil {
		return dbe(err)
	}
	return nil
}

const retrySQL = `
UPDATE radish_tasks
SET status = 'retry',
	errors = jsonb(:errors),
	visible_at = :visibleAt,
	modified = :modified
WHERE id=:id
`

// Mark a task as retryable with the given errors and backoff delay.
func (b *Broker) Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
	now := time.Now()
	params := []any{
		sql.Named("id", id),
		sql.Named("errors", errors),
		sql.Named("visibleAt", now.Add(delay)),
		sql.Named("modified", now),
	}

	if _, err = b.Exec(ctx, retrySQL, params...); err != nil {
		return dbe(err)
	}
	return nil
}

const successSQL = `
UPDATE radish_tasks
SET status = 'succeeded',
	visible_at = NULL,
	finished = datetime('now'),
	modified = datetime('now')
WHERE id=:id
`

// Mark a task as succeeded with no additional errors.
func (b *Broker) Success(ctx context.Context, id int64) (err error) {
	params := []any{
		sql.Named("id", id),
	}
	if _, err = b.Exec(ctx, successSQL, params...); err != nil {
		return dbe(err)
	}
	return nil
}

const vacuumSQL = `
DELETE FROM radish_tasks WHERE
	status IN ('succeeded', 'failed', 'cancelled')
	AND finished < :retention
`

// Cleans up any completed tasks that are older than the retention period.
func (b *Broker) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	params := []any{
		sql.Named("retention", time.Now().Add(-retention)),
	}

	if _, err = b.Exec(ctx, vacuumSQL, params...); err != nil {
		return dbe(err)
	}
	return nil
}
