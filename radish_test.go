package radish_test

import (
	"errors"
	"sync"
	"testing"

	. "github.com/kansaslabs/radish"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/require"
)

func TestRadishQueue(t *testing.T) {
	wg := new(sync.WaitGroup)
	wg.Add(8)

	good := &testTask{wg: wg, name: "good"}
	bad := &testTask{wg: wg, name: "bad", onHandle: func(id uuid.UUID, params []byte) error { return errors.New("whoops!") }}

	queue, err := New(&Config{Workers: 2}, good, bad)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := queue.Delay(good.Name(), nil, nil, nil)
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		_, err := queue.Delay(bad.Name(), nil, nil, nil)
		require.NoError(t, err)
	}

	wg.Wait()
	require.Equal(t, int32(5), good.handled)
	require.Equal(t, int32(5), good.successes)
	require.Equal(t, int32(0), good.failures)
	require.Equal(t, int32(3), bad.handled)
	require.Equal(t, int32(0), bad.successes)
	require.Equal(t, int32(3), bad.failures)
}

func TestRadishScaling(t *testing.T) {
	// Create a queue with 4 workers
	radish, err := New(&Config{Workers: 4})
	require.NoError(t, err)
	require.Equal(t, 4, radish.NumWorkers())

	// Set the workers to 10 (increase number of workers)
	require.NoError(t, radish.SetWorkers(10))
	require.Equal(t, 10, radish.NumWorkers())

	// Set the workers to the same number of workers (should be noop)
	require.NoError(t, radish.SetWorkers(10))
	require.Equal(t, 10, radish.NumWorkers())

	// Set the workers to 3 (decrease number of workers)
	require.NoError(t, radish.SetWorkers(3))
	require.Equal(t, 3, radish.NumWorkers())

	// Set an invalid number of workers
	// TODO: can we set 0 workers on the queue?
	require.EqualError(t, radish.SetWorkers(-8), "[5] cannot set number of workers <0")
	require.Equal(t, 3, radish.NumWorkers())

	// Add 2 workers
	require.NoError(t, radish.AddWorkers(2))
	require.Equal(t, 5, radish.NumWorkers())

	// Add 0 workers (should be a noop)
	require.NoError(t, radish.AddWorkers(0))
	require.Equal(t, 5, radish.NumWorkers())

	// Add an invalid number of workers
	require.EqualError(t, radish.AddWorkers(-16), "[5] cannot add negative workers, use RemoveWorkers")
	require.Equal(t, 5, radish.NumWorkers())

	// Remove 1 worker
	require.NoError(t, radish.RemoveWorkers(1))
	require.Equal(t, 4, radish.NumWorkers())

	// Remove 0 workers (should be a noop)
	require.NoError(t, radish.RemoveWorkers(0))
	require.Equal(t, 4, radish.NumWorkers())

	// Remove an invalid number of workers
	require.EqualError(t, radish.RemoveWorkers(-6), "[5] cannot remove negative workers, use AddWorkers")
	require.Equal(t, 4, radish.NumWorkers())

	// Remove more workers than exist
	require.EqualError(t, radish.RemoveWorkers(87), "[5] cannot remove 87 workers, only 4 currently running")
	require.Equal(t, 4, radish.NumWorkers())
}
