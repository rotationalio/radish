package tests

import (
	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
)

type TaskAssertions struct {
	require *require.Assertions
	actual  *models.TaskMeta
}

func RequireTaskFactory(require *require.Assertions) func(func() (*models.TaskMeta, error)) *TaskAssertions {
	return func(task func() (*models.TaskMeta, error)) *TaskAssertions {
		actual, err := task()
		require.NoError(err, "unexpected error occurred while getting task")
		require.NotNil(actual, "expected task to be non-nil but it is nil")
		return RequireTask(require, actual)
	}
}

func RequireTask(require *require.Assertions, actual *models.TaskMeta) *TaskAssertions {
	return &TaskAssertions{
		require: require,
		actual:  actual,
	}
}

// Expect asserts the task state matches the expected values.
// Does not check errors or any of the timestamp fields.
// Does require a valid created and modified timestamp.
func (a TaskAssertions) Expect(id int64, kind string, status status.Status, payload []byte, attempts int16) {
	a.require.Equal(id, a.actual.ID, "task ID mismatch")
	a.require.Equal(kind, a.actual.Kind, "task kind mismatch")
	a.require.Equal(status, a.actual.Status, "task status mismatch")
	a.require.JSONEq(string(payload), string(a.actual.Payload), "task payload mismatch")
	a.require.Equal(attempts, a.actual.Attempts, "task attempts mismatch")
	a.require.False(a.actual.Created.IsZero(), "task created timestamp is zero")
	a.require.False(a.actual.Modified.IsZero(), "task modified timestamp is zero")
}

func (a TaskAssertions) HasVisibleAt() {
	a.require.False(a.actual.VisibleAt.Time.IsZero(), "expected task to have visible at timestamp but it is zero valued")
}

func (a TaskAssertions) NoVisibleAt() {
	a.require.False(a.actual.VisibleAt.Valid, "expected task to have no visible at timestamp but it is valid")
}

func (a TaskAssertions) HasLastAttempt() {
	a.require.False(a.actual.LastAttempt.Time.IsZero(), "expected task to have last attempt timestamp but it is zero valued")
}

func (a TaskAssertions) NoLastAttempt() {
	a.require.False(a.actual.LastAttempt.Valid, "expected task to have no last attempt timestamp but it is valid")
}

func (a TaskAssertions) HasFinished() {
	a.require.False(a.actual.Finished.Time.IsZero(), "expected task to have finished timestamp but it is zero valued")
}

func (a TaskAssertions) NoFinished() {
	a.require.False(a.actual.Finished.Valid, "expected task to have no finished timestamp but it is valid")
}

// Asserts that the given task has no errors.
func (a TaskAssertions) NoErrors() {
	if len(a.actual.Errors) > 0 {
		a.require.FailNow("expected no errors but task has %d errors: %v", len(a.actual.Errors), a.actual.Errors)
	}
}

// Assert that the given task has the specified error.
func (a TaskAssertions) HasError(errorString string) {
	if len(a.actual.Errors) == 0 {
		a.require.FailNow("expected task to have an error but no errors are on task")
	}

	for _, e := range a.actual.Errors {
		if e.Error == errorString {
			return
		}
	}
	a.require.FailNow("expected task to have error %s but no error matches: %v", errorString, a.actual.Errors)
}
