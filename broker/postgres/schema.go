package postgres

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

const radishSchemaSQL = `
BEGIN;

-- Create the radish_status type if it does not exist.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'radish_status') THEN
        CREATE TYPE radish_status AS ENUM (
            'pending',
            'retry',
            'scheduled',
            'running',
            'succeeded',
            'failed',
            'cancelled',
        );
    END IF;
END $$;

-- Create the radish_tasks table if it does not exist.
CREATE TABLE IF NOT EXISTS radish_tasks (
    id              BIGSERIAL PRIMARY KEY,
    kind            VARCHAR(255) NOT NULL,
    status          radish_status NOT NULL DEFAULT 'pending',
    payload         JSONB NOT NULL DEFAULT '{}',
    attempts        SMALLINT NOT NULL DEFAULT 0,
    errors          JSONB NOT NULL DEFAULT '[]',
    visible_at      TIMESTAMPTZ DEFAULT NULL,
    last_attempt    TIMESTAMPTZ DEFAULT NULL,
    finished        TIMESTAMPTZ DEFAULT NULL,
    created         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modified        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
`

func initializeSchema(ctx context.Context, conn *sql.DB) (err error) {
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

	// Execute the schema.
	if _, err = cur.ExecContext(ctx, radishSchemaSQL); err != nil {
		return err
	}

	return nil
}
