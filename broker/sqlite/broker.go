package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/broker/options"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
	"go.rtnl.ai/x/dsn"
)

type Broker struct {
	mu sync.RWMutex
	db *sql.DB

	// Prepared statements
	infoSQL              *sql.Stmt
	enqueueSQL           *sql.Stmt
	scheduleSQL          *sql.Stmt
	dequeueSelectSQL     *sql.Stmt
	dequeueUpdateSQL     *sql.Stmt
	cancelSQL            *sql.Stmt
	failedSQL            *sql.Stmt
	retrySQL             *sql.Stmt
	successSQL           *sql.Stmt
	vacuumSQL            *sql.Stmt
	queueSizeSQL         *sql.Stmt
	queueStatusCountsSQL *sql.Stmt
	queueKindsCountsSQL  *sql.Stmt
	queueTimeRangeSQL    *sql.Stmt
	timeSeriesSQL        *sql.Stmt
}

const countSQL = `
SELECT COUNT(*) FROM radish_tasks
`

const listSQL = `
SELECT id, kind, status, json_extract(payload, '$') as payload, attempts, json_extract(errors, '$') as errors, visible_at, last_attempt, finished, created, modified
FROM radish_tasks
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

	if err = tx.QueryRow(countSQL+filter.Clause(dsn.SQLite3), filter.Params()...).Scan(&count); err != nil {
		return nil, err
	}

	if rows, err = tx.Query(listSQL+filter.Clause(dsn.SQLite3), filter.Params()...); err != nil {
		return nil, err
	}

	return cursor.New(tx, rows, count), nil
}

const infoSQL = `
SELECT id, kind, status, json_extract(payload, '$') as payload, attempts, json_extract(errors, '$') as errors, visible_at, last_attempt, finished, created, modified
FROM radish_tasks WHERE id=:id;
`

// Get information about a task by its id.
func (b *Broker) Info(ctx context.Context, id int64) (task *models.TaskMeta, err error) {
	task = &models.TaskMeta{}
	row := b.infoSQL.QueryRowContext(ctx, sql.Named("id", id))

	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

const enqueueSQL = `
INSERT INTO radish_tasks (kind, payload) VALUES (:kind, jsonb(:payload)) RETURNING id;
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

	row := b.enqueueSQL.QueryRowContext(ctx, sql.Named("kind", kind), sql.Named("payload", payload))
	if err = row.Scan(&id); err != nil {
		return 0, dbe(err)
	}

	if err = row.Err(); err != nil {
		return 0, dbe(err)
	}

	return id, nil
}

// Because SQLite3 does not support arrays for parameters, this query has to be manually
// constructed and cannot be used as a prepared statement.
const countKindsSQL = `
SELECT COUNT(*) FROM radish_tasks
	WHERE status IN ('pending', 'running', 'scheduled', 'retry') AND
	kind
`

func (b *Broker) enqueueOnlyOne(ctx context.Context, kind string, payload []byte, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	// Create the kind parameters for the query.
	clause, params := opts.KindsSQLite3Params()
	countQuery := strings.TrimSpace(countKindsSQL) + " " + clause

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	if err = tx.QueryRow(countQuery, params...).Scan(&count); err != nil {
		return 0, dbe(err)
	}

	if count > 0 {
		return 0, errors.ErrHighlander
	}

	stmt := tx.StmtContext(ctx, b.enqueueSQL)
	if err = stmt.QueryRow(sql.Named("kind", kind), sql.Named("payload", payload)).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

// Because SQLite3 does not support arrays for parameters, this query has to be manually
// constructed and cannot be used as a prepared statement.
const cancelKindsSQL = `
UPDATE radish_tasks
SET status = 'cancelled',
	visible_at = NULL,
	finished = datetime('now'),
	modified = datetime('now')
WHERE status in ('pending', 'scheduled', 'retry') AND kind
`

func (b *Broker) enqueueOnlyOneReplace(ctx context.Context, kind string, payload []byte, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	// Create the kind parameters for the query.
	clause, params := opts.KindsSQLite3Params()
	cancelQuery := strings.TrimSpace(cancelKindsSQL) + " " + clause

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(cancelQuery, params...); err != nil {
		return 0, dbe(err)
	}

	stmt := tx.StmtContext(ctx, b.enqueueSQL)
	if err = stmt.QueryRow(sql.Named("kind", kind), sql.Named("payload", payload)).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

const scheduleSQL = `
INSERT INTO radish_tasks (kind, status, payload, visible_at)
	VALUES (:kind, 'scheduled', jsonb(:payload), :visibleAt) RETURNING id;
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

	row := b.scheduleSQL.QueryRowContext(ctx, sql.Named("kind", kind), sql.Named("payload", payload), sql.Named("visibleAt", executeAfter))
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

	// Create the kind parameters for the query.
	clause, params := opts.KindsSQLite3Params()
	countQuery := strings.TrimSpace(countKindsSQL) + " " + clause

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var count int64
	if err = tx.QueryRow(countQuery, params...).Scan(&count); err != nil {
		return 0, dbe(err)
	}

	if count > 0 {
		return 0, errors.ErrHighlander
	}

	stmt := tx.StmtContext(ctx, b.scheduleSQL)
	if err = stmt.QueryRow(sql.Named("kind", kind), sql.Named("payload", payload), sql.Named("visibleAt", executeAfter)).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
}

func (b *Broker) scheduleOnlyOneReplace(ctx context.Context, kind string, payload []byte, executeAfter time.Time, opts *options.Options) (id int64, err error) {
	if len(opts.Kinds) == 0 {
		return 0, errors.ErrKindsRequired
	}

	// Create the kind parameters for the query.
	clause, params := opts.KindsSQLite3Params()
	cancelQuery := strings.TrimSpace(cancelKindsSQL) + " " + clause

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: false}); err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(cancelQuery, params...); err != nil {
		return 0, dbe(err)
	}

	stmt := tx.StmtContext(ctx, b.scheduleSQL)
	if err = stmt.QueryRow(sql.Named("kind", kind), sql.Named("payload", payload), sql.Named("visibleAt", executeAfter)).Scan(&id); err != nil {
		return 0, dbe(err)
	}

	return id, tx.Commit()
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
	dequeueSelect := tx.StmtContext(ctx, b.dequeueSelectSQL)
	if err = task.Scan(dequeueSelect.QueryRow()); err != nil {
		return nil, dbe(err)
	}

	task.Status = status.Running
	task.Attempts++

	now := time.Now()
	task.VisibleAt = sql.NullTime{Time: now.Add(ttl), Valid: true}
	task.LastAttempt = sql.NullTime{Time: now, Valid: true}
	task.Modified = now

	dequeueUpdate := tx.StmtContext(ctx, b.dequeueUpdateSQL)
	params := []any{
		sql.Named("id", task.ID),
		sql.Named("status", task.Status),
		sql.Named("attempts", task.Attempts),
		sql.Named("visibleAt", task.VisibleAt),
		sql.Named("lastAttempt", task.LastAttempt),
		sql.Named("modified", task.Modified),
	}
	if _, err = dequeueUpdate.Exec(params...); err != nil {
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
	var result sql.Result
	if result, err = b.cancelSQL.ExecContext(ctx, sql.Named("id", id)); err != nil {
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
	if _, err = b.failedSQL.ExecContext(ctx, sql.Named("id", id), sql.Named("errors", errors)); err != nil {
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
	if _, err = b.retrySQL.ExecContext(ctx, sql.Named("id", id), sql.Named("errors", errors), sql.Named("visibleAt", now.Add(delay)), sql.Named("modified", now)); err != nil {
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
	if _, err = b.successSQL.ExecContext(ctx, sql.Named("id", id)); err != nil {
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
	if _, err = b.vacuumSQL.ExecContext(ctx, sql.Named("retention", time.Now().Add(-retention))); err != nil {
		return dbe(err)
	}
	return nil
}

const queueSizeSQL = `
	SELECT COUNT(*) FROM radish_tasks
	WHERE status IN ('pending', 'running', 'scheduled', 'retry')
`

func (b *Broker) QueueSize(ctx context.Context) (count int64, err error) {
	if err = b.queueSizeSQL.QueryRowContext(ctx).Scan(&count); err != nil {
		return 0, dbe(err)
	}
	return count, nil
}

const (
	queueStatusCountsSQL = "SELECT status, COUNT(*) FROM radish_tasks GROUP BY status;"
	queueKindsCountsSQL  = "SELECT kind, COUNT(*) FROM radish_tasks GROUP BY kind;"
	queueTimeRangeSQL    = "SELECT MIN(created), MAX(created), MAX(visible_at) FROM radish_tasks;"
)

func (b *Broker) QueueStatus(ctx context.Context) (out *models.QueueStatus, err error) {
	out = &models.QueueStatus{
		Statuses: make(map[status.Status]int64, 7),
		Kinds:    make(map[string]int64),
	}

	var tx *sql.Tx
	if tx, err = b.BeginTx(ctx, &sql.TxOptions{ReadOnly: true}); err != nil {
		return nil, err
	}
	defer tx.Rollback()

	queueStatusCounts := tx.StmtContext(ctx, b.queueStatusCountsSQL)
	defer queueStatusCounts.Close()

	var statusRows *sql.Rows
	if statusRows, err = queueStatusCounts.QueryContext(ctx); err != nil {
		return nil, dbe(err)
	}
	defer statusRows.Close()

	for statusRows.Next() {
		var state status.Status
		var count int64
		if err = statusRows.Scan(&state, &count); err != nil {
			return nil, dbe(err)
		}

		out.Statuses[state] = count
		switch {
		case state <= status.Running:
			out.Awaiting += count
		case state > status.Running:
			out.Completed += count
		}
	}

	queueKindsCounts := tx.StmtContext(ctx, b.queueKindsCountsSQL)
	defer queueKindsCounts.Close()

	var kindRows *sql.Rows
	if kindRows, err = queueKindsCounts.QueryContext(ctx); err != nil {
		return nil, dbe(err)
	}
	defer kindRows.Close()

	for kindRows.Next() {
		var kind string
		var count int64
		if err = kindRows.Scan(&kind, &count); err != nil {
			return nil, dbe(err)
		}

		out.Kinds[kind] = count
	}

	queueTimeRange := tx.StmtContext(ctx, b.queueTimeRangeSQL)
	defer queueTimeRange.Close()

	if err = queueTimeRange.QueryRowContext(ctx).Scan(&out.Earliest, &out.Latest, &out.ScheduledUntil); err != nil {
		return nil, dbe(err)
	}

	return out, nil
}

const timeSeriesSQL = `
WITH bins AS (
	SELECT value AS period
	FROM generate_series(
		unixepoch(:after),
		unixepoch(:before),
		:interval
	)
)
SELECT
	datetime(b.period, 'unixepoch') AS timestamp,
	COUNT(t.id) AS enqueued
FROM bins b
LEFT JOIN radish_tasks t
	unixepoch(t.created) >= b.period AND unixepoch(t.created) < (b.period + :interval)
GROUP BY
	b.period
ORDER BY
	b.period;
`

// NOTE: cannot specify a zero-valued start time (e.g. after) or specify an after time
// that is after the end time (e.g. before). The minimum interval is 1 minute. The
// interval will also be truncated to the nearest second for sqlite3 support.
func (b *Broker) TimeSeries(ctx context.Context, after, before time.Time, interval time.Duration) (series models.Series, err error) {
	// If before is zero, set it to the current time.
	if before.IsZero() {
		before = time.Now()
	}

	// Time series constraints.
	if after.IsZero() || after.After(before) || interval < 1*time.Minute {
		return nil, errors.ErrInvalidTimeRange
	}

	// Truncate the interval to the nearest second.
	interval = interval.Truncate(time.Second)
	seconds := int64(interval.Seconds())

	var rows *sql.Rows
	if rows, err = b.timeSeriesSQL.QueryContext(ctx, sql.Named("after", after), sql.Named("before", before), sql.Named("interval", seconds)); err != nil {
		return nil, dbe(err)
	}
	defer rows.Close()

	series = make(models.Series, 0)
	for rows.Next() {
		period := &models.Period{}
		if err = period.Scan(rows); err != nil {
			return nil, dbe(err)
		}
		series = append(series, period)
	}

	return series, nil
}

func (b *Broker) prepareStatements() (err error) {
	if b.infoSQL, err = b.db.Prepare(infoSQL); err != nil {
		return fmt.Errorf("failed to prepare info statement: %w", err)
	}
	if b.enqueueSQL, err = b.db.Prepare(enqueueSQL); err != nil {
		return fmt.Errorf("failed to prepare enqueue statement: %w", err)
	}
	if b.scheduleSQL, err = b.db.Prepare(scheduleSQL); err != nil {
		return fmt.Errorf("failed to prepare schedule statement: %w", err)
	}
	if b.dequeueSelectSQL, err = b.db.Prepare(dequeueSelectSQL); err != nil {
		return fmt.Errorf("failed to prepare dequeue statement: %w", err)
	}
	if b.dequeueUpdateSQL, err = b.db.Prepare(dequeueUpdateSQL); err != nil {
		return fmt.Errorf("failed to prepare dequeue update statement: %w", err)
	}
	if b.cancelSQL, err = b.db.Prepare(cancelSQL); err != nil {
		return fmt.Errorf("failed to prepare cancel statement: %w", err)
	}
	if b.failedSQL, err = b.db.Prepare(failSQL); err != nil {
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
	if b.queueStatusCountsSQL, err = b.db.Prepare(queueStatusCountsSQL); err != nil {
		return fmt.Errorf("failed to prepare queue status counts statement: %w", err)
	}
	if b.queueKindsCountsSQL, err = b.db.Prepare(queueKindsCountsSQL); err != nil {
		return fmt.Errorf("failed to prepare queue kinds counts statement: %w", err)
	}
	if b.queueTimeRangeSQL, err = b.db.Prepare(queueTimeRangeSQL); err != nil {
		return fmt.Errorf("failed to prepare queue time range statement: %w", err)
	}
	if b.timeSeriesSQL, err = b.db.Prepare(timeSeriesSQL); err != nil {
		return fmt.Errorf("failed to prepare time series statement: %w", err)
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
	if b.scheduleSQL != nil {
		b.scheduleSQL.Close()
	}
	if b.dequeueSelectSQL != nil {
		b.dequeueSelectSQL.Close()
	}
	if b.dequeueUpdateSQL != nil {
		b.dequeueUpdateSQL.Close()
	}
	if b.cancelSQL != nil {
		b.cancelSQL.Close()
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
	if b.queueStatusCountsSQL != nil {
		b.queueStatusCountsSQL.Close()
	}
	if b.queueKindsCountsSQL != nil {
		b.queueKindsCountsSQL.Close()
	}
	if b.queueTimeRangeSQL != nil {
		b.queueTimeRangeSQL.Close()
	}
	if b.timeSeriesSQL != nil {
		b.timeSeriesSQL.Close()
	}
}
