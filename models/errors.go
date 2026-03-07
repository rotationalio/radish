package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type AttemptErrors []AttemptError

// An error from a single task attempt that failed due to error or panic.
type AttemptError struct {
	// The attempt number on which the error occurred.
	Attempt int `json:"attempt"`

	// The error that occurred.
	Error string `json:"error"`

	// Trace contains a stack trace from a job that panicked.
	// In the case of a non-panic or error produced as a stuck task, this value will
	// be an empty string
	Trace string `json:"trace"`

	// Timestamp that the error occurred
	Timestamp time.Time `json:"timestamp"`
}

//===========================================================================
// Database Interaction
//===========================================================================

func (s *AttemptErrors) Scan(src interface{}) (err error) {
	switch x := src.(type) {
	case nil:
		*s = make(AttemptErrors, 0)
		return nil
	case []byte:
		buf := make([]byte, len(x))
		copy(buf, x)
		return json.Unmarshal(buf, s)
	default:
		return fmt.Errorf("cannot scan %T into attempt errors", src)
	}
}

func (s AttemptErrors) Value() (driver.Value, error) {
	return json.Marshal(s)
}
