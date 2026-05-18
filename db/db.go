package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"go.rtnl.ai/x/dsn"
)

const Postgres = "postgres"

var (
	mu   sync.RWMutex
	conn *sql.DB
)

func Use(db *sql.DB) (err error) {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		return ErrAlreadyConnected
	}
	conn = db

	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()

	if err = conn.PingContext(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("could not ping database: %w", err)
	}

	// Initialize the schema.
	if err = initializeSchema(ctx); err != nil {
		return err
	}

	return nil
}

func Connect(databaseURL string) (err error) {
	mu.Lock()
	defer mu.Unlock()

	if conn != nil {
		return ErrAlreadyConnected
	}

	var uri *dsn.DSN
	if uri, err = dsn.Parse(databaseURL); err != nil {
		return err
	}

	if uri.Provider != Postgres {
		return fmt.Errorf("unsupported database provider: %s", uri.Provider)
	}

	var (
		connStr string
		opts    *PostgresOptions
	)
	if connStr, opts, err = ConnectionOptions(*uri); err != nil {
		return err
	}

	if conn, err = sql.Open(Postgres, connStr); err != nil {
		return err
	}

	// Apply any options to the connection string.
	conn.SetMaxIdleConns(opts.MaxIdleConns)
	conn.SetMaxOpenConns(opts.MaxOpenConns)
	conn.SetConnMaxLifetime(opts.ConnMaxLifetime)
	conn.SetConnMaxIdleTime(opts.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()

	if err = conn.PingContext(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("could not ping database: %w", err)
	}

	// Initialize the schema.
	if err = initializeSchema(ctx); err != nil {
		return err
	}

	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if conn == nil {
		return nil
	}

	err := conn.Close()
	conn = nil
	return err
}

func Exec(ctx context.Context, query string, args ...any) (result sql.Result, err error) {
	mu.RLock()
	defer mu.RUnlock()

	if conn == nil {
		return nil, ErrNotConnected
	}

	if result, err = conn.ExecContext(ctx, query, args...); err != nil {
		return nil, dbe(err)
	}
	return result, nil
}

func QueryRow(ctx context.Context, query string, args ...any) (row *sql.Row, err error) {
	mu.RLock()
	defer mu.RUnlock()

	if conn == nil {
		return nil, ErrNotConnected
	}

	if row = conn.QueryRowContext(ctx, query, args...); err != nil {
		return nil, dbe(err)
	}
	return row, nil
}

func Begin() (*sql.Tx, error) {
	mu.RLock()
	defer mu.RUnlock()
	if conn == nil {
		return nil, ErrNotConnected
	}
	return conn.Begin()
}

func BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	mu.RLock()
	defer mu.RUnlock()
	if conn == nil {
		return nil, ErrNotConnected
	}
	return conn.BeginTx(ctx, opts)
}
