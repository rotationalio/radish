package radish

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//============================================================================
// Test Tasks
//============================================================================

type SleepTask struct {
	Duration time.Duration `json:"duration"`
}

func (t *SleepTask) Kind() string {
	return "sleep"
}

type SortTask struct {
	Numbers []int `json:"numbers"`
}

func (t *SortTask) Kind() string {
	return "sort"
}

func (t *SortTask) KindAliases() []string {
	return []string{"sort-numbers", "sort-integers"}
}

type RandomFailureTask struct {
	Probability float64 `json:"probability"`
}

func (t *RandomFailureTask) Kind() string {
	return "failchance"
}

func (t *RandomFailureTask) KindAliases() []string {
	return []string{"random-failure", "random-error"}
}

type MockTask struct{}

func (t *MockTask) Kind() string {
	return "mock"
}

func (t *MockTask) KindAliases() []string {
	// intentionally duplicates the SortTask kind aliases
	return []string{"sleep", "sort", "failchance"}
}

//============================================================================
// Interface Implementations
//============================================================================

func TestTaskInterface(t *testing.T) {
	require.Implements(t, (*Task)(nil), new(SleepTask))
	require.NotImplements(t, (*TaskWithAliases)(nil), new(SleepTask))
	require.Implements(t, (*TaskWithAliases)(nil), new(SortTask))
	require.Implements(t, (*Task)(nil), new(RandomFailureTask))
	require.Implements(t, (*TaskWithAliases)(nil), new(RandomFailureTask))
}
