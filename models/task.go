package models

import (
	"database/sql"
	"time"

	"go.rtnl.ai/radish/status"
)

// TaskMeta is the properties of a task that are persisted in the database.
type TaskMeta struct {
	// ID of the task.
	ID int64

	// Kind uniquely identifies the type of task and which worker is responsible for processing it.// Kind uniquely identifies the type of task and which worker is responsible for processing it.
	Kind string

	// Status of the task.
	Status status.Status

	// JSON encoded payload of the task.
	Payload []byte

	// Current attempt of the task. Attempts are inserted at 0, the number is
	// incremented to 1 the first time it is worked, and it may be incremented further
	// if it errors.
	Attempts int16

	// Errors for each attempt that failed, ordered from earliest to latest.
	Errors AttemptErrors

	// Timestamp of when the task becomes visible to workers. This is used both for
	// backoff delays in retries and for scheduling tasks in the future.
	VisibleAt sql.NullTime

	// Timestamp of when the last attempt to work the task was started.
	LastAttempt sql.NullTime

	// Timestamp of when the task was completed successfully or failed.
	Finished sql.NullTime

	// Timestamp Metadata for record auditing and tracking.
	Created  time.Time
	Modified time.Time
}
