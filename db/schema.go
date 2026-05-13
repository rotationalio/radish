package db

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"go.rtnl.ai/x/rlog"
)

const (
	acquireMigrationLockSQL = `SELECT pg_advisory_lock($1);`
	releaseMigrationLockSQL = `SELECT pg_advisory_unlock($1);`
	initializeTimeout       = 90 * time.Second
	AdvisoryLockID          = int64(4006367007158143198)
)

func initializeSchema(ctx context.Context) (err error) {
	// Acquire a single connection so we can acquire an advisory lock.
	var cur *sql.Conn
	if cur, err = conn.Conn(ctx); err != nil {
		return err
	}
	defer cur.Close()

	// Acquire the advisory lock.
	if _, err = cur.ExecContext(ctx, acquireMigrationLockSQL, AdvisoryLockID); err != nil {
		return err
	}

	// Ensure the advisory lock is released.
	defer func() {
		if _, err := conn.ExecContext(ctx, releaseMigrationLockSQL, AdvisoryLockID); err != nil {
			rlog.ErrorAttrs(ctx, "could not release advisory lock", slog.Any("err", err))
		}
	}()

	// Load the schema.
	var schema string
	if schema, err = Query("schema"); err != nil {
		return err
	}

	// Execute the schema.
	if _, err = cur.ExecContext(ctx, schema); err != nil {
		return err
	}

	return nil
}
