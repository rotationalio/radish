package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/lib/pq"
	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/broker/options"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/x/dsn"
)

type Broker struct {
	mu sync.RWMutex
	db *sql.DB

	// Prepared statements
	infoSQL        *sql.Stmt
	countKindsSQL  *sql.Stmt
	enqueueSQL     *sql.Stmt
	scheduleSQL    *sql.Stmt
	dequeueSQL     *sql.Stmt
	cancelSQL      *sql.Stmt
	cancelKindsSQL *sql.Stmt
	failedSQL      *sql.Stmt
	retrySQL       *sql.Stmt
	successSQL     *sql.Stmt
	vacuumSQL      *sql.Stmt
	queueSizeSQL   *sql.Stmt
	lockTableSQL   *sql.Stmt
}

const countSQL = `
-- Count tasks appending an optional filter at the end of the query.
SELECT COUNT(*) FROM radish_tasks
`

const listSQL = `
-- List tasks appending an optional filter at the end of the query.
SELECT * FROM radish_tasks
`

// List tasks with the given filter.
// Callers must close the cursor when they are done with it to ensure the transaction
// is closed and rolled back. Callers do not have access to the transaction.
// NOTE: because of the dynamic filter we cannot use a prepared statement for this query.
func (b *Broker) List(ctx context.Context, filter *cursor.Filter) (tasks *cursor.Cursor, err error) {
	var (
		tx    *sql.Tx
		count int64
		rows  *sql.Rows
	)

	if filter == nil {
		filter = cursor.Where()
	}

	// NOTE: do not defer the rollback, that will happen when the cursor is closed.
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: true}); err != nil {
		return nil, err
	}

	if err = tx.QueryRow(countSQL+filter.Clause(dsn.Postgres), filter.Params()...).Scan(&count); err != nil {
		return nil, err
	}

	if rows, err = tx.Query(listSQL+filter.Clause(dsn.Postgres), filter.Params()...); err != nil {
		return nil, err
	}

	return cursor.New(tx, rows, count), nil
}

const infoSQL = `
-- Get information about a task by its id.
SELECT * FROM radish_tasks WHERE id = $1;
`

// Get information about a task by its id.
func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	task = &models.TaskMeta{}
	row := b.infoSQL.QueryRowContext(ctx, id)

	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const enqueueSQL = `
-- Enqueue a task with the given kind and payload.
-- Parameter: the kind of task to enqueue and its JSON encoded payload.
INSERT INTO radish_tasks (kind, payload) VALUES ($1, $2) RETURNING id;
`

// Enqueue a task with the given kind and payload.
func (b *Broker) Enqueue(ctx context.Context, kind string, payload []byte, opts *options.Options) (id int64, err error) {
	if opts != nil {
		switch {
		case opts.OnlyOne:
			return b.enqueueOnlyOne(ctx, kind, payload, opts)
		case opts.OnlyOneReplace:
			return b.enqueueOnlyOneReplace(ctx, kind, payload, opts)
		}
	}

	if err = b.enqueueSQL.QueryRowContext(ctx, kind, payload).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, nil
}

const countKindsSQL = `
-- Count the number of tasks with the given kind or kind alias.
SELECT COUNT(*) FROM radish_tasks
	WHERE kind = ANY($1) AND
	status = ANY(ARRAY['pending', 'running', 'scheduled', 'retry']::radish_status[]);
`

func (b *Broker) enqueueOnlyOne(ctx context.Context, kind string, payload []byte, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	countStmt := tx.StmtContext(ctx, b.countKindsSQL)
	if err = countStmt.QueryRow(pq.Array(opts.Kinds)).Scan(&count); err != nil {
		return 0, dbe(err)
	}

	if count > 0 {
		return 0, errors.ErrHighlander
	}

	stmt := tx.StmtContext(ctx, b.enqueueSQL)
	if err = stmt.QueryRow(kind, payload).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

const lockTableSQL = `
-- Blocks inserts, updates, and deletes on the radish_tasks table but allows concurrent selects.
LOCK TABLE radish_tasks IN EXCLUSIVE MODE;
`

const cancelKindsSQL = `
-- Mark all tasks with the given kinds as cancelled with no additional errors.
UPDATE radish_tasks
SET status = 'cancelled',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE kind = ANY($1) AND status = ANY(ARRAY['pending', 'scheduled', 'retry']::radish_status[]);
`

func (b *Broker) enqueueOnlyOneReplace(ctx context.Context, kind string, payload []byte, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	lockStmt := tx.StmtContext(ctx, b.lockTableSQL)
	if _, err = lockStmt.Exec(); err != nil {
		return 0, dbe(err)
	}

	cancelStmt := tx.StmtContext(ctx, b.cancelKindsSQL)
	if _, err = cancelStmt.Exec(pq.Array(opts.Kinds)); err != nil {
		return 0, dbe(err)
	}

	stmt := tx.StmtContext(ctx, b.enqueueSQL)
	if err = stmt.QueryRow(kind, payload).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

const scheduleSQL = `
-- Schedule a task with the given kind and payload to be executed later.
-- Parameter: the kind of task to schedule and its JSON encoded payload and the timestamp when it should be executed after.
INSERT INTO radish_tasks (kind, status, payload, visible_at)
    VALUES ($1, 'scheduled', $2, $3) RETURNING id;
`

// Schedule a task with the given kind and payload to be executed later.
func (b *Broker) Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time, opts *options.Options) (id int64, err error) {
	if opts != nil {
		switch {
		case opts.OnlyOne:
			return b.scheduleOnlyOne(ctx, kind, payload, executeAfter, opts)
		case opts.OnlyOneReplace:
			return b.scheduleOnlyOneReplace(ctx, kind, payload, executeAfter, opts)
		}
	}

	row := b.scheduleSQL.QueryRowContext(ctx, kind, payload, executeAfter)
	if err = row.Scan(&id); err != nil {
		return 0, dbe(err)
	}

	if err = row.Err(); err != nil {
		return 0, dbe(err)
	}

	return id, nil
}

func (b *Broker) scheduleOnlyOne(ctx context.Context, kind string, payload []byte, executeAfter time.Time, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	countStmt := tx.StmtContext(ctx, b.countKindsSQL)
	if err = countStmt.QueryRow(pq.Array(opts.Kinds)).Scan(&count); err != nil {
		return 0, dbe(err)
	}

	if count > 0 {
		return 0, errors.ErrHighlander
	}

	stmt := tx.StmtContext(ctx, b.scheduleSQL)
	if err = stmt.QueryRow(kind, payload, executeAfter).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

func (b *Broker) scheduleOnlyOneReplace(ctx context.Context, kind string, payload []byte, executeAfter time.Time, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	lockStmt := tx.StmtContext(ctx, b.lockTableSQL)
	if _, err = lockStmt.Exec(); err != nil {
		return 0, dbe(err)
	}

	cancelStmt := tx.StmtContext(ctx, b.cancelKindsSQL)
	if _, err = cancelStmt.Exec(pq.Array(opts.Kinds)); err != nil {
		return 0, dbe(err)
	}

	stmt := tx.StmtContext(ctx, b.scheduleSQL)
	if err = stmt.QueryRow(kind, payload, executeAfter).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

const dequeueSQL = `
-- Dequeue the next task from the queue, skipping over locked rows.
-- Parameter: the visibility timeout for the task in seconds.
WITH next_task AS (
    SELECT id FROM radish_tasks
    WHERE
        status = 'pending' OR
        (status = ANY(ARRAY['running', 'scheduled', 'retry']::radish_status[]) AND visible_at <= NOW())
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
	task = &models.TaskMeta{}
	row := b.dequeueSQL.QueryRowContext(ctx, ttl.Seconds())

	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const cancelSQL = `
-- Mark all tasks with the given kind as cancelled with no additional errors.
UPDATE radish_tasks
SET status = 'cancelled',
    visible_at = NULL,
    finished = NOW(),
    modified = NOW()
WHERE id = $1 AND status = ANY(ARRAY['pending', 'scheduled', 'retry']::radish_status[]);
`

// Mark a task as cancelled with no additional errors.
// NOTE: if the task is already in progress or has been completed an error will be
// returned that the task cannot be cancelled.
func (b *Broker) Cancel(ctx context.Context, id int64) (err error) {
	var result sql.Result
	if result, err = b.cancelSQL.ExecContext(ctx, id); err != nil {
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
	if _, err = b.failedSQL.ExecContext(ctx, id, errors); err != nil {
		return dbe(err)
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
	if _, err = b.retrySQL.ExecContext(ctx, id, errors, delay.Seconds()); err != nil {
		return dbe(err)
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
	if _, err = b.successSQL.ExecContext(ctx, id); err != nil {
		return dbe(err)
	}
	return nil
}

const vacuumSQL = `
-- Cleanup any completed tasks that are older than the retention period.
-- Parameter: the retention period in seconds.
DELETE FROM radish_tasks WHERE
    status = ANY(ARRAY['succeeded', 'failed', 'cancelled']::radish_status[]) AND
    finished < NOW() - make_interval(secs := $1);
`

// Cleans up any completed tasks that are older than the retention period.
func (b *Broker) Vacuum(ctx context.Context, retention time.Duration) (err error) {
	if _, err = b.vacuumSQL.ExecContext(ctx, retention.Seconds()); err != nil {
		return dbe(err)
	}
	return nil
}

const queueSizeSQL = `
	SELECT COUNT(*) FROM radish_tasks
	WHERE status = ANY(ARRAY['pending', 'running', 'scheduled', 'retry']::radish_status[])
`

func (b *Broker) QueueSize(ctx context.Context) (count int64, err error) {
	if err = b.queueSizeSQL.QueryRowContext(ctx).Scan(&count); err != nil {
		return 0, dbe(err)
	}
	return count, nil
}

func (b *Broker) prepareStatements() (err error) {
	if b.infoSQL, err = b.db.Prepare(infoSQL); err != nil {
		return fmt.Errorf("failed to prepare info statement: %w", err)
	}
	if b.enqueueSQL, err = b.db.Prepare(enqueueSQL); err != nil {
		return fmt.Errorf("failed to prepare enqueue statement: %w", err)
	}
	if b.countKindsSQL, err = b.db.Prepare(countKindsSQL); err != nil {
		return fmt.Errorf("failed to prepare count kinds statement: %w", err)
	}
	if b.scheduleSQL, err = b.db.Prepare(scheduleSQL); err != nil {
		return fmt.Errorf("failed to prepare schedule statement: %w", err)
	}
	if b.dequeueSQL, err = b.db.Prepare(dequeueSQL); err != nil {
		return fmt.Errorf("failed to prepare dequeue statement: %w", err)
	}
	if b.cancelSQL, err = b.db.Prepare(cancelSQL); err != nil {
		return fmt.Errorf("failed to prepare cancel statement: %w", err)
	}
	if b.cancelKindsSQL, err = b.db.Prepare(cancelKindsSQL); err != nil {
		return fmt.Errorf("failed to prepare cancel kinds statement: %w", err)
	}
	if b.failedSQL, err = b.db.Prepare(failedSQL); err != nil {
		return fmt.Errorf("failed to prepare failed statement: %w", err)
	}
	if b.retrySQL, err = b.db.Prepare(retrySQL); err != nil {
		return fmt.Errorf("failed to prepare retry statement: %w", err)
	}
	if b.successSQL, err = b.db.Prepare(successSQL); err != nil {
		return fmt.Errorf("failed to prepare success statement: %w", err)
	}
	if b.vacuumSQL, err = b.db.Prepare(vacuumSQL); err != nil {
		return fmt.Errorf("failed to prepare vacuum statement: %w", err)
	}
	if b.queueSizeSQL, err = b.db.Prepare(queueSizeSQL); err != nil {
		return fmt.Errorf("failed to prepare queue size statement: %w", err)
	}
	if b.lockTableSQL, err = b.db.Prepare(lockTableSQL); err != nil {
		return fmt.Errorf("failed to prepare lock table statement: %w", err)
	}
	return nil
}

func (b *Broker) closeStatements() {
	if b.infoSQL != nil {
		b.infoSQL.Close()
	}
	if b.enqueueSQL != nil {
		b.enqueueSQL.Close()
	}
	if b.countKindsSQL != nil {
		b.countKindsSQL.Close()
	}
	if b.scheduleSQL != nil {
		b.scheduleSQL.Close()
	}
	if b.dequeueSQL != nil {
		b.dequeueSQL.Close()
	}
	if b.cancelSQL != nil {
		b.cancelSQL.Close()
	}
	if b.cancelKindsSQL != nil {
		b.cancelKindsSQL.Close()
	}
	if b.failedSQL != nil {
		b.failedSQL.Close()
	}
	if b.retrySQL != nil {
		b.retrySQL.Close()
	}
	if b.successSQL != nil {
		b.successSQL.Close()
	}
	if b.vacuumSQL != nil {
		b.vacuumSQL.Close()
	}
	if b.queueSizeSQL != nil {
		b.queueSizeSQL.Close()
	}
	if b.lockTableSQL != nil {
		b.lockTableSQL.Close()
	}
}
