package radish

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish/internal/worker"
	"go.rtnl.ai/radish/models"
)

type RandomTask struct {
	Sleep time.Duration `json:"sleep"`
	Value int           `json:"value"`
}

func (t *RandomTask) Kind() string {
	return "random"
}

type RandomWorker struct {
	WorkerDefaults[*RandomTask]
}

func (w *RandomWorker) Do(ctx context.Context, task *TaskInfo[*RandomTask]) error {
	time.Sleep(task.Task.Sleep)
	task.Task.Value = rand.Intn(math.MaxInt)
	return nil
}

func TestFactory(t *testing.T) {
	workers := &Workers{}
	AddWorker(workers, new(RandomWorker))

	// Simulate making a bunch of tasks with the factory
	tasks := make([]worker.Worker, 16)
	for i := range tasks {
		var err error
		tasks[i], err = workers.workers["random"].factory.Make(&models.TaskMeta{
			Kind:    "random",
			Payload: []byte(fmt.Sprintf(`{"sleep": %d, "value": 0}`, randomSleep())),
		})
		require.NoError(t, err)
	}

	// Execute all the tasks
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task worker.Worker) {
			defer wg.Done()
			require.NoError(t, task.UnmarshalTask())
			require.NoError(t, task.Do(context.Background()))
		}(task)
	}

	wg.Wait()

	// Check the results
	for i, task := range tasks {
		require.Nil(t, task.Retry())
		require.Zero(t, task.Timeout())

		for j, other := range tasks {
			if i == j {
				continue
			}

			wtask := task.(*wrappedWorker[*RandomTask])
			otask := other.(*wrappedWorker[*RandomTask])

			require.NotEqual(t, wtask.task.Task, otask.task.Task)
			require.NotEqual(t, wtask.task.Task.Value, otask.task.Task.Value)
		}
	}

}

func randomSleep() time.Duration {
	return time.Duration(rand.Intn(2000)+500) * time.Millisecond
}
