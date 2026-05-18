package db

import (
	"context"
	"database/sql"
	"embed"
	"strings"
	"time"

	"go.rtnl.ai/radish/models"
)

// SQL contains the query statements for the database, written in SQL files
// for ease of management for larger queries.
//
//go:embed sql/*.sql
var queries embed.FS

// Load a query from the SQL directory.
func Query(name string) (string, error) {
	if !strings.HasSuffix(name, ".sql") {
		name += ".sql"
	}

	query, err := queries.ReadFile("sql/" + name)
	if err != nil {
		return "", err
	}
	return string(query), nil
}

// Enqueue a task with the given kind and payload.
func Enqueue(ctx context.Context, kind string, payload []byte) (id int64, err error) {
	var query string
	if query, err = Query("enqueue"); err != nil {
		return 0, err
	}

	var row *sql.Row
	if row, err = QueryRow(ctx, query, kind, payload); err != nil {
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

// Schedule a task with the given kind and payload to be executed later.
func Schedule(ctx context.Context, kind string, payload []byte, executeAfter time.Time) (id int64, err error) {
	var query string
	if query, err = Query("schedule"); err != nil {
		return 0, err
	}

	var row *sql.Row
	if row, err = QueryRow(ctx, query, kind, payload, executeAfter); err != nil {
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

// Mark a task as cancelled with no additional errors.
// NOTE: if the task is already in progress or has been completed an error will be
// returned that the task cannot be cancelled.
func Cancel(ctx context.Context, id int64) (err error) {
	var query string
	if query, err = Query("cancel"); err != nil {
		return err
	}

	var result sql.Result
	if result, err = Exec(ctx, query, id); err != nil {
		return err
	}

	var rows int64
	if rows, err = result.RowsAffected(); err != nil {
		return dbe(err)
	}

	if rows == 0 {
		return ErrTaskNotCancelable
	}
	return nil
}

// Dequeue the next task from the queue, skipping over locked rows.
// The TTL is the time that the dequeueing entity has to complete the task before it
// may be considered failed and re-assigned to a different worker.
func Dequeue(ctx context.Context, ttl time.Duration) (task *models.TaskMeta, err error) {
	var query string
	if query, err = Query("dequeue"); err != nil {
		return nil, err
	}

	var row *sql.Row
	if row, err = QueryRow(ctx, query, ttl.Seconds()); err != nil {
		return nil, err
	}

	task = &models.TaskMeta{}
	if err = task.Scan(row); err != nil {
		return nil, dbe(err)
	}

	return task, nil
}

// Mark a task as failed with the given errors.
func Fail(ctx context.Context, id int64, errors models.AttemptErrors) (err error) {
	var query string
	if query, err = Query("failed"); err != nil {
		return err
	}

	if _, err = Exec(ctx, query, id, errors); err != nil {
		return err
	}
	return nil
}

// Mark a task as retryable with the given errors and backoff delay.
func Retry(ctx context.Context, id int64, errors models.AttemptErrors, delay time.Duration) (err error) {
	var query string
	if query, err = Query("retry"); err != nil {
		return err
	}

	if _, err = Exec(ctx, query, id, errors, delay.Seconds()); err != nil {
		return err
	}
	return nil
}

// Mark a task as succeeded with no additional errors.
func Success(ctx context.Context, id int64) (err error) {
	var query string
	if query, err = Query("success"); err != nil {
		return err
	}

	if _, err = Exec(ctx, query, id); err != nil {
		return err
	}
	return nil
}

// Cleans up any completed tasks that are older than the retention period.
func Vacuum(ctx context.Context, retention time.Duration) (err error) {
	var query string
	if query, err = Query("vacuum"); err != nil {
		return err
	}

	if _, err = Exec(ctx, query, retention.Seconds()); err != nil {
		return err
	}

	return nil
}
