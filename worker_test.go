package radish

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//============================================================================
// Test Workers
//============================================================================

type SleepWorker struct {
	WorkerDefaults[*SleepTask]
}

func (w *SleepWorker) Do(ctx context.Context, task *TaskInfo[*SleepTask]) error {
	time.Sleep(task.Task.Duration)
	return nil
}

type SortWorker struct {
	WorkerDefaults[*SortTask]
}

func (w *SortWorker) Do(ctx context.Context, task *TaskInfo[*SortTask]) error {
	sort.Ints(task.Task.Numbers)
	return nil
}

type RandomFailureWorker struct {
	WorkerDefaults[*RandomFailureTask]
}

func (w *RandomFailureWorker) Do(ctx context.Context, task *TaskInfo[*RandomFailureTask]) error {
	if rand.Float64() < task.Task.Probability {
		return errors.New("random failure")
	}
	return nil
}

type MockWorker struct {
	WorkerDefaults[*MockTask]
	OnDo func(ctx context.Context, task *TaskInfo[*MockTask]) error
}

func (w *MockWorker) Do(ctx context.Context, task *TaskInfo[*MockTask]) error {
	if w.OnDo != nil {
		return w.OnDo(ctx, task)
	}
	return nil
}

//============================================================================
// Interface Implementations
//============================================================================

func TestWorkerInterface(t *testing.T) {
	require.Implements(t, (*Worker[*SleepTask])(nil), new(SleepWorker))
	require.Implements(t, (*Worker[*SortTask])(nil), new(SortWorker))
	require.Implements(t, (*Worker[*RandomFailureTask])(nil), new(RandomFailureWorker))
}

func TestWorkFunc(t *testing.T) {
	worker := WorkFunc(func(ctx context.Context, task *TaskInfo[*SortTask]) error {
		sort.Ints(task.Task.Numbers)
		return nil
	})
	require.Implements(t, (*Worker[*SortTask])(nil), worker)
	require.NotImplements(t, (*Worker[*SleepTask])(nil), worker)

	info := &TaskInfo[*SortTask]{
		Task: &SortTask{
			Numbers: []int{3, 1, 2},
		},
	}
	require.NoError(t, worker.Do(context.Background(), info))
	require.Equal(t, []int{1, 2, 3}, info.Task.Numbers)
}

//============================================================================
// Workers Tests
//============================================================================

func TestRegister(t *testing.T) {
	conf := mockConfig(t)

	t.Run("HappyPath", func(t *testing.T) {
		tasks, err := New(conf)

		require.NoError(t, err)
		require.False(t, tasks.IsRunning())
		require.NoError(t, Register(tasks, new(SleepWorker)))
		require.NoError(t, Register(tasks, new(SortWorker)))
		require.NoError(t, Register(tasks, new(RandomFailureWorker)))

		workers := tasks.Workers()
		require.Equal(t, 7, workers.Len())
		require.True(t, workers.Has("sleep"))
		require.True(t, workers.Has("sort"))
		require.True(t, workers.Has("failchance"))
	})

	t.Run("Running", func(t *testing.T) {
		// Cannot register tasks while the radish instance is running.
		tasks, err := New(conf)
		require.NoError(t, err)

		require.NoError(t, Register(tasks, new(SleepWorker)))
		tasks.MarkRunning()

		require.True(t, tasks.IsRunning())
		err = Register(tasks, new(SortWorker))
		require.ErrorIs(t, err, ErrRunning)
	})
}

func TestWorkers(t *testing.T) {
	t.Run("HappyPath", func(t *testing.T) {
		workers := &Workers{}
		AddWorker(workers, new(SleepWorker))
		AddWorker(workers, new(SortWorker))
		AddWorker(workers, new(RandomFailureWorker))

		require.Equal(t, 7, workers.Len())
		require.True(t, workers.Has("sleep"))
		require.True(t, workers.Has("sort"))
		require.True(t, workers.Has("sort-numbers"))
		require.True(t, workers.Has("sort-integers"))
		require.True(t, workers.Has("failchance"))
		require.True(t, workers.Has("random-failure"))
		require.True(t, workers.Has("random-error"))
		require.False(t, workers.Has("unknown"))
	})

	t.Run("Duplicate", func(t *testing.T) {
		workers := &Workers{}
		AddWorker(workers, new(SleepWorker))
		require.Error(t, AddWorkerSafe(workers, new(SleepWorker)))
	})

	t.Run("DuplicateAlias", func(t *testing.T) {
		workers := &Workers{}
		AddWorker(workers, new(SortWorker))
		require.Error(t, AddWorkerSafe(workers, new(MockWorker)))
	})

	t.Run("DuplicatePanics", func(t *testing.T) {
		workers := &Workers{}
		AddWorker(workers, new(SortWorker))
		require.Panics(t, func() {
			AddWorker(workers, new(SortWorker))
		})
	})
}
