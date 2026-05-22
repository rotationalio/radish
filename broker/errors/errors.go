package errors

import (
	"errors"
)

var (
	// Database constraint errors
	ErrBusy              = errors.New("database is busy and cannot acquire lock")
	ErrAlreadyExists     = errors.New("record already exists in the database")
	ErrConstraint        = errors.New("a database constraint was violated")
	ErrDBReference       = errors.New("missing id of foreign key reference")
	ErrDeleteRestricted  = errors.New("cannot delete record because other records depend on it")
	ErrNotFound          = errors.New("record not found in the database")
	ErrNotNull           = errors.New("cannot set a required field to null")
	ErrReadOnly          = errors.New("database or transaction is read-only")
	ErrNotConnected      = errors.New("database connection not established")
	ErrTaskNotCancelable = errors.New("task cannot be cancelled")
)

var (
	Is   = errors.Is
	As   = errors.As
	Join = errors.Join
)
