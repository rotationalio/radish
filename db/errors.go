package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var (
	// Database constraint errors
	ErrAlreadyExists    = errors.New("record already exists in the database")
	ErrConstraint       = errors.New("a database constraint was violated")
	ErrDBReference      = errors.New("missing id of foreign key reference")
	ErrDeleteRestricted = errors.New("cannot delete record because other records depend on it")
	ErrNotFound         = errors.New("record not found in the database")
	ErrNotNull          = errors.New("cannot set a required field to null")
	ErrReadOnly         = errors.New("database or transaction is read-only")
	ErrAlreadyConnected = errors.New("database connection already established")
	ErrNotConnected     = errors.New("database connection not established")
)

func dbe(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrAlreadyExists
		case "23503":
			return ErrDBReference
		case "23502":
			return ErrNotNull
		case "23000", "23514":
			return ErrConstraint
		case "23001":
			return ErrDeleteRestricted
		case "25006":
			return ErrReadOnly
		}
	}

	return fmt.Errorf("pgx error: %w", err)
}
