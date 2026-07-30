package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/x/dsn"
)

// Use an existing databasse connection rather than connecting to the database. A
// postgres connection is expected. Use will ping the database and initialize the schema.
func Use(db *sql.DB) (broker *Broker, err error) {
	if err = initializeDB(db); err != nil {
		return nil, err
	}
	return &Broker{db: db}, nil
}

// Connect to a new database using the provided DSN. The DSN is parsed for connection
// options and the database is pinged and initialized. A new postgres broker is returned.
func Connect(uri *dsn.DSN) (broker *Broker, err error) {
	if uri.Provider != dsn.Postgres {
		return nil, fmt.Errorf("unsupported database provider: %s", uri.Provider)
	}

	var (
		connStr string
		opts    *Options
	)
	if connStr, opts, err = ConnectionOptions(uri); err != nil {
		return nil, err
	}

	broker = &Broker{}
	if broker.db, err = sql.Open(dsn.Postgres, connStr); err != nil {
		return nil, err
	}
	// Apply any options to the connection string.
	broker.db.SetMaxIdleConns(opts.MaxIdleConns)
	broker.db.SetMaxOpenConns(opts.MaxOpenConns)
	broker.db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	broker.db.SetConnMaxIdleTime(opts.ConnMaxIdleTime)

	if err = initializeDB(broker.db); err != nil {
		return nil, err
	}

	if err = broker.prepareStatements(); err != nil {
		return nil, err
	}

	return broker, nil
}

func initializeDB(db *sql.DB) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()

	// Make sure the database is reachable.
	if err = db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return fmt.Errorf("could not ping database: %w", err)
	}

	// Initialize the schema.
	if err = initializeSchema(ctx, db); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return err
	}
	return nil
}

func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Allow calling Close multiple times without error.
	if b.db == nil {
		return nil
	}

	// Close the prepared statements.
	b.closeStatements()

	// Close the database connection and set the database connection to nil.
	err := b.db.Close()
	b.db = nil
	return err
}

func (b *Broker) Exec(ctx context.Context, query string, args ...any) (result sql.Result, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.db == nil {
		return nil, errors.ErrNotConnected
	}

	if result, err = b.db.ExecContext(ctx, query, args...); err != nil {
		return nil, dbe(err)
	}
	return result, nil
}

func (b *Broker) QueryRow(ctx context.Context, query string, args ...any) (row *sql.Row, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.db == nil {
		return nil, errors.ErrNotConnected
	}

	row = b.db.QueryRowContext(ctx, query, args...)
	return row, nil
}

func (b *Broker) Begin() (*sql.Tx, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.db == nil {
		return nil, errors.ErrNotConnected
	}
	return b.db.Begin()
}

func (b *Broker) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.db == nil {
		return nil, errors.ErrNotConnected
	}
	return b.db.BeginTx(ctx, opts)
}

func dbe(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return errors.ErrNotFound
	}

	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errors.ErrAlreadyExists
		case "23503":
			return errors.ErrDBReference
		case "23502":
			return errors.ErrNotNull
		case "23000", "23514":
			return errors.ErrConstraint
		case "23001":
			return errors.ErrDeleteRestricted
		case "25006":
			return errors.ErrReadOnly
		}
	}

	return fmt.Errorf("pgx error: %w", err)
}
