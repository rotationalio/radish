package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/x/dsn"
	_ "modernc.org/sqlite"
)

// Connect to a new sqlite database using the provided DSN.
func Connect(uri *dsn.DSN) (broker *Broker, err error) {
	if uri.Provider != dsn.SQLite3 {
		return nil, fmt.Errorf("unsupported database provider: %s", uri.Provider)
	}

	if uri.Path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	broker = &Broker{}
	ctx, cancel := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancel()

	if broker.db, err = sql.Open("sqlite", sqliteDSN(uri)); err != nil {
		return nil, err
	}

	if err = broker.db.PingContext(ctx); err != nil {
		return nil, err
	}

	if err = initializeSchema(ctx, broker.db); err != nil {
		return nil, err
	}

	if err = broker.prepareStatements(); err != nil {
		return nil, err
	}

	return broker, nil
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

func sqliteDSN(uri *dsn.DSN) string {
	params := url.Values{}
	params.Set("_txlock", "immediate")

	for key, value := range uri.Options {
		params.Set(key, value)
	}

	return fmt.Sprintf("file:%s?%s", uri.Path, params.Encode())
}

func dbe(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return errors.ErrNotFound
	}

	// sqliteErr := &sqlite.Error{}
	// if errors.As(err, sqliteErr) {
	// 	// See: https://sqlite.org/rescode.html
	// 	switch sqliteErr.Code() {
	// 	case 5:
	// 		return errors.ErrBusy
	// 	case 8:
	// 		return errors.ErrReadOnly
	// 	case 12:
	// 		return errors.ErrNotNull
	// 	case 19:
	// 		return errors.ErrConstraint
	// 	case 101:
	// 		return errors.ErrNotFound
	// 	}
	// }

	return fmt.Errorf("sqlite3 error: %w", err)
}
