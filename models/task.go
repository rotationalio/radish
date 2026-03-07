package models

import (
	"database/sql"
	"time"

	"go.rtnl.ai/radish/status"
	"go.rtnl.ai/ulid"
)

// TaskMeta is the properties of a task that are persisted in the database.
type TaskMeta struct {
	// ID of the task. A ULID that guarantees montonically increasing values.
	ID ulid.ULID

	// Current attempt of the task. Attempts are inserted at 0, the number is
	// incremented to 1 the first time it is worked, and it may be incremented further
	// if it errors.
	Attempt int

	// Set of client IDs that have worked this job.
	AttemptedBy []string

	// Errors for each attempt that failed, ordered from earliest to latest.
	Errors AttemptErrors

	// Timestamp of when the task was completed successfully or failed.
	Finished sql.NullTime

	// Kind uniquely identifies the type of task and which worker is responsible for processing it.
	Kind string

	// Timestamp of when the last attempt to work the task was started.
	LastAttempt sql.NullTime

	// JSON encoded payload of the task.
	Payload []byte

	// The maximum number of attempts the task will be tried before it errors for the
	// last time and will no longer be attempted.
	Retries int

	// Status of the task.
	Status status.Status

	// Timestamp of when the task becomes visible to workers. This is used both for
	// backoff delays in retries and for scheduling tasks in the future.
	VisibleAt sql.NullTime

	// Timestamp Metadata
	Created  time.Time
	Modified time.Time
}
