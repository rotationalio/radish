package sqlite

import (
	"context"
	"database/sql"
	"time"
)

const initializeTimeout = 90 * time.Second

const radishSchemaSQL = `
PRAGMA journal_mode=DELETE;
PRAGMA synchronous=FULL;
PRAGMA foreign_keys=OFF;
PRAGMA busy_timeout=5000;

BEGIN;

CREATE TABLE IF NOT EXISTS radish_tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    kind            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    payload         BLOB NOT NULL DEFAULT (jsonb('{}')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    errors          BLOB NOT NULL DEFAULT (jsonb('[]')),
    visible_at      DATETIME DEFAULT NULL,
    last_attempt    DATETIME DEFAULT NULL,
    finished        DATETIME DEFAULT NULL,
    created         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    modified        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMIT;
`

func initializeSchema(ctx context.Context, conn *sql.DB) (err error) {
	if _, err = conn.ExecContext(ctx, radishSchemaSQL); err != nil {
		return err
	}
	return nil
}
