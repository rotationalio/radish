package tests

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/broker/errors"
	"go.rtnl.ai/radish/broker/options"
	"go.rtnl.ai/radish/models"
	"go.rtnl.ai/radish/status"
)

const (
	TTL         = 60 * time.Second
	testKind    = "test"
	testPayload = `{"color":"red"}`
)

// The BrokerTestSuite is a suite of tests that covers the broker interface and the
// expected invariants of the broker. These tests are generalized so that they can be
// used to test any broker implementation. Use the suite.Suite interfaces to prepare
// the test environment prior to running the tests.
//
// TODO: increase coverage of the broker interface and expected invariants.
type BrokerTestSuite struct {
	suite.Suite
	Broker broker.Broker
	cancel []context.CancelFunc
}

func New(b broker.Broker) *BrokerTestSuite {
	return &BrokerTestSuite{
		Broker: b,
	}
}

func (s *BrokerTestSuite) Context() (ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	s.cancel = append(s.cancel, cancel)

	return ctx
}

func (s *BrokerTestSuite) TearDownTest() {
	for _, cancel := range s.cancel {
		cancel()
	}
	s.cancel = nil
}

func (s *BrokerTestSuite) TestListEmpty() {
	ctx := s.Context()
	require := s.Require()

	tasks, err := s.Broker.List(ctx, nil)
	require.NoError(err, "unable to list tasks")
	defer tasks.Close() //nolint:errcheck

	require.Zero(tasks.Count(), "expected 0 tasks but got %d", tasks.Count())

	meta, err := tasks.List()
	require.NoError(err, "unable to list tasks")
	require.Empty(meta, "expected no tasks but got %d", len(meta))
}

func (s *BrokerTestSuite) TestList() {
	// Before this test starts enqueue tasks with several kinds and statuses.
	ctx := s.Context()
	require := s.Require()
	configs := []EnqueueConfig{
		{
			Kind:       "test",
			NPending:   16,
			NRetry:     8,
			NSuccess:   4,
			NFailed:    1,
			NScheduled: 3,
			NCancelled: 2,
		},
		{
			Kind:       "foo",
			NPending:   8,
			NRetry:     1,
			NSuccess:   122,
			NFailed:    0,
			NScheduled: 8,
			NCancelled: 1,
		},
	}

	total := int64(0)
	for _, config := range configs {
		err := config.Enqueue(ctx, s.Broker)
		require.NoError(err, "unable to enqueue tasks")
		total += config.Total()
	}

	s.Run("All", func() {
		tasks, err := s.Broker.List(ctx, nil)
		require.NoError(err, "unable to list tasks")
		defer tasks.Close() //nolint:errcheck

		require.Equal(total, tasks.Count(), "expected %d tasks but got %d", total, tasks.Count())
	})

	s.Run("Filtered", func() {
		tasks, err := s.Broker.List(ctx, cursor.Where().Kinds("test").States(status.Pending))
		require.NoError(err, "unable to list tasks")
		defer tasks.Close() //nolint:errcheck

		require.Equal(int64(16), tasks.Count(), "expected %d tasks but got %d", 16, tasks.Count())
	})
}

func (s *BrokerTestSuite) TestDequeueNotFound() {
	ctx := s.Context()
	require := s.Require()

	// Dequeue a task from an empty database; should return ErrNotFound.
	task, err := s.Broker.Dequeue(ctx, TTL)
	require.ErrorIs(err, errors.ErrNotFound)
	require.Nil(task)

	// Enqueue several tasks and mark them as succeeded, failed, and cancelled.
	for i := 0; i < 64; i++ {
		id, err := s.Broker.Enqueue(ctx, testKind, []byte(testPayload), nil)
		require.NoError(err, "unable to enqueue task")
		require.NotZero(id, "unable to enqueue task")

		spin := rand.Float64()
		switch {
		case spin < 0.10:
			err = s.Broker.Fail(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}})
			require.NoError(err, "unable to mark task as failed")
		case spin < 0.20:
			err = s.Broker.Cancel(ctx, id)
			require.NoError(err, "unable to mark task as cancelled")
		default:
			err = s.Broker.Success(ctx, id)
			require.NoError(err, "unable to mark task as succeeded")
		}
	}

	// Dequeue a task; should return ErrNotFound.
	task, err = s.Broker.Dequeue(ctx, TTL)
	require.ErrorIs(err, errors.ErrNotFound)
	require.Nil(task)
}

func (s *BrokerTestSuite) TestEnqueueDequeueSingleSuccessfulTask() {
	ctx := s.Context()
	require := s.Require()
	requireTask := RequireTaskFactory(require)

	// Enqueue a task
	id, err := s.Broker.Enqueue(ctx, testKind, []byte(testPayload), nil)
	require.NoError(err, "unable to enqueue task")
	require.NotZero(id, "unable to enqueue task")

	// Ensure the task is in the database
	task := requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Info(ctx, id)
	})
	task.Expect(id, testKind, status.Pending, []byte(testPayload), 0)
	task.NoErrors()
	task.NoVisibleAt()
	task.NoLastAttempt()
	task.NoFinished()

	// Dequeue the task
	task = requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Dequeue(ctx, TTL)
	})
	task.Expect(id, testKind, status.Running, []byte(testPayload), 1)
	task.HasVisibleAt()
	task.HasLastAttempt()
	task.NoFinished()

	// Mark the task as succeeded
	err = s.Broker.Success(ctx, id)
	require.NoError(err, "unable to mark task as succeeded")

	// Check the task status
	task = requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Info(ctx, id)
	})
	task.Expect(id, testKind, status.Succeeded, []byte(testPayload), 1)
	task.NoErrors()
	task.NoVisibleAt()
	task.HasLastAttempt()
	task.HasFinished()
}

func (s *BrokerTestSuite) TestScheduleDequeueSingleFailedTask() {
	ctx := s.Context()
	require := s.Require()
	requireTask := RequireTaskFactory(require)

	// Schedule a task
	id, err := s.Broker.Enqueue(ctx, testKind, []byte(testPayload), nil)
	require.NoError(err, "unable to schedule task")
	require.NotZero(id, "unable to schedule task")

	// Ensure the task is in the database
	task := requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Info(ctx, id)
	})
	task.Expect(id, testKind, status.Pending, []byte(testPayload), 0)
	task.NoErrors()
	task.NoVisibleAt()
	task.NoLastAttempt()
	task.NoFinished()

	// Dequeue the task
	task = requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Dequeue(ctx, TTL)
	})
	task.Expect(id, testKind, status.Running, []byte(testPayload), 1)
	task.HasVisibleAt()
	task.HasLastAttempt()
	task.NoFinished()

	// Mark the task as failed
	err = s.Broker.Fail(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}})
	require.NoError(err, "unable to mark task as failed")

	// Check the task status
	task = requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Info(ctx, id)
	})
	task.Expect(id, testKind, status.Failed, []byte(testPayload), 1)
	task.HasError("test")
	task.NoVisibleAt()
	task.HasLastAttempt()
	task.HasFinished()

	// Dequeue the task
	task = requireTask(func() (*models.TaskMeta, error) {
		return s.Broker.Info(ctx, id)
	})
	task.Expect(id, testKind, status.Failed, []byte(testPayload), 1)
	task.HasError("test")
	task.NoVisibleAt()
	task.HasLastAttempt()
	task.HasFinished()
}

func (s *BrokerTestSuite) TestEnqueueOnlyOne() {
	ctx := s.Context()
	require := s.Require()

	// Enqueue multiple tasks with different kinds with the OnlyOne option
	s.Run("Kind", func() {
		kinds := []string{"connor", "duncan", "kurgan", "heather", "kastagir"}
		tasks := make([]int64, 0, len(kinds))
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind, []byte(testPayload), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
			tasks = append(tasks, id)
		}

		// Enqueueing a second time should error
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind, []byte(testPayload), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.ErrorIs(err, errors.ErrHighlander)
			require.Zero(id)
		}

		// Cancel the task should allow enqueuing it again
		for _, id := range tasks {
			err := s.Broker.Cancel(ctx, id)
			require.NoError(err, "unable to cancel task")
		}

		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind, []byte(testPayload), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
		}
	})

	// NOTE: databases are nto reset between subtests, so we need to use different
	// aliases and kinds that will not conflict with other subtests.
	s.Run("Aliases", func() {
		kinds := [][]string{
			{"macleod", "wyatt"},
			{"sutherland", "stirling"},
			{"macdonald", "mackenzie"},
		}
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind[0], []byte(testPayload), &options.Options{OnlyOne: true, Kinds: kind})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
		}

		// Enqueueing with the alias a second time should error
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind[1], []byte(testPayload), &options.Options{OnlyOne: true, Kinds: kind})
			require.ErrorIs(err, errors.ErrHighlander)
			require.Zero(id)
		}
	})
}

func (s *BrokerTestSuite) TestEnqueueOnlyOneReplace() {
	ctx := s.Context()
	require := s.Require()

	// Enqueue multiple tasks with different kinds with the OnlyOneReplace option
	s.Run("Kind", func() {
		kinds := []string{"connor", "duncan", "kurgan", "heather", "kastagir"}
		tasks := make([]int64, 0, len(kinds))

		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind, []byte(testPayload), &options.Options{OnlyOneReplace: true, Kinds: []string{kind}})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
			tasks = append(tasks, id)
		}

		// Enqueueing with the same kind should replace the existing task
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind, []byte(testPayload), &options.Options{OnlyOneReplace: true, Kinds: []string{kind}})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
		}

		// The old tasks should be marked as cancelled
		for _, id := range tasks {
			task, err := s.Broker.Info(ctx, id)
			require.NoError(err, "unable to get task info")
			require.Equal(status.Cancelled, task.Status, "expected task to be marked as cancelled")
		}
	})

	s.Run("Aliases", func() {
		kinds := [][]string{
			{"macleod", "wyatt"},
			{"sutherland", "stirling"},
			{"macdonald", "mackenzie"},
		}
		tasks := make([]int64, 0, len(kinds))
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind[0], []byte(testPayload), &options.Options{OnlyOneReplace: true, Kinds: kind})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
			tasks = append(tasks, id)
		}

		// Enqueueing with the kind alias should replace the existing task
		for _, kind := range kinds {
			id, err := s.Broker.Enqueue(ctx, kind[1], []byte(testPayload), &options.Options{OnlyOneReplace: true, Kinds: kind})
			require.NoError(err, "unable to enqueue task")
			require.NotZero(id, "unable to enqueue task")
		}

		// The old tasks should be marked as cancelled
		for _, id := range tasks {
			task, err := s.Broker.Info(ctx, id)
			require.NoError(err, "unable to get task info")
			require.Equal(status.Cancelled, task.Status, "expected task to be marked as cancelled")
		}
	})
}

func (s *BrokerTestSuite) TestScheduleOnlyOne() {
	ctx := s.Context()
	require := s.Require()

	// Schedule multiple tasks with different kinds with the OnlyOne option
	s.Run("Kind", func() {
		kinds := []string{"connor", "duncan", "kurgan", "heather", "kastagir"}
		tasks := make([]int64, 0, len(kinds))

		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind, []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
			tasks = append(tasks, id)
		}

		// Scheduling a second time should error
		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind, []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.ErrorIs(err, errors.ErrHighlander)
			require.Zero(id)
		}

		// Cancel the task should allow scheduling it again
		for _, id := range tasks {
			err := s.Broker.Cancel(ctx, id)
			require.NoError(err, "unable to cancel task")
		}

		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind, []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOne: true, Kinds: []string{kind}})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
		}
	})

	s.Run("Aliases", func() {
		kinds := [][]string{
			{"macleod", "wyatt"},
			{"sutherland", "stirling"},
			{"macdonald", "mackenzie"},
		}

		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind[0], []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOne: true, Kinds: kind})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
		}

		// Scheduling with the kind alias should return an error
		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind[1], []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOne: true, Kinds: kind})
			require.ErrorIs(err, errors.ErrHighlander)
			require.Zero(id)
		}

	})
}

func (s *BrokerTestSuite) TestScheduleOnlyOneReplace() {
	ctx := s.Context()
	require := s.Require()

	// Schedule multiple tasks with different kinds with the OnlyOneReplace option
	s.Run("Kind", func() {
		kinds := []string{"connor", "duncan", "kurgan", "heather", "kastagir"}
		tasks := make([]int64, 0, len(kinds))
		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind, []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOneReplace: true, Kinds: []string{kind}})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
			tasks = append(tasks, id)
		}

		// Scheduling with the same kind should replace the existing task
		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind, []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOneReplace: true, Kinds: []string{kind}})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
		}

		// The old tasks should be marked as cancelled
		for _, id := range tasks {
			task, err := s.Broker.Info(ctx, id)
			require.NoError(err, "unable to get task info")
			require.Equal(status.Cancelled, task.Status, "expected task to be marked as cancelled")
		}
	})

	s.Run("Aliases", func() {
		kinds := [][]string{
			{"macleod", "wyatt"},
			{"sutherland", "stirling"},
			{"macdonald", "mackenzie"},
		}
		tasks := make([]int64, 0, len(kinds))
		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind[0], []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOneReplace: true, Kinds: kind})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")
			tasks = append(tasks, id)
		}

		for _, kind := range kinds {
			id, err := s.Broker.Schedule(ctx, kind[0], []byte(testPayload), time.Now().Add(randDelay()), &options.Options{OnlyOneReplace: true, Kinds: kind})
			require.NoError(err, "unable to schedule task")
			require.NotZero(id, "unable to schedule task")

			task, err := s.Broker.Info(ctx, id)
			require.NoError(err, "unable to get task info")
			require.Equal(status.Scheduled, task.Status, "expected task to be marked as scheduled, actual status is %s", task.Status)
		}

		// The old tasks should be marked as cancelled
		for _, id := range tasks {
			task, err := s.Broker.Info(ctx, id)
			require.NoError(err, "unable to get task info")
			require.Equal(status.Cancelled, task.Status, "expected task to be marked as cancelled")
		}
	})
}

func (s *BrokerTestSuite) TestQueueSize() {
	ctx := s.Context()
	require := s.Require()

	count, err := s.Broker.QueueSize(ctx)
	require.NoError(err, "unable to get queue size")
	require.Equal(int64(0), count, "expected queue size to be 0")

	// Enqueue some tasks
	configs := []EnqueueConfig{
		{
			Kind:       "test",
			NPending:   16,
			NRetry:     8,
			NSuccess:   4,
			NFailed:    1,
			NScheduled: 3,
			NCancelled: 2,
		},
		{
			Kind:       "foo",
			NPending:   8,
			NRetry:     1,
			NSuccess:   122,
			NFailed:    0,
			NScheduled: 8,
			NCancelled: 1,
		},
	}

	for _, config := range configs {
		err := config.Enqueue(ctx, s.Broker)
		require.NoError(err, "unable to enqueue tasks")
	}

	count, err = s.Broker.QueueSize(ctx)
	require.NoError(err, "unable to get queue size")
	require.Equal(configs[0].QueueSize()+configs[1].QueueSize(), count, "expected queue size to be the sum of the enqueued tasks")
}

func (s *BrokerTestSuite) TestQueueStatus() {
	ctx := s.Context()
	require := s.Require()

	s.Run("Empty", func() {
		info, err := s.Broker.QueueStatus(ctx)
		require.NoError(err, "unable to get queue status")
		require.NotNil(info)

		require.Empty(info.Statuses, "statuses should be empty")
		require.Empty(info.Kinds, "kinds should be empty")
		require.Zero(info.Awaiting, "awaiting should be 0")
		require.Zero(info.Completed, "completed should be 0")
		require.False(info.Earliest.Valid, "earliest should be null")
		require.False(info.Latest.Valid, "latest should be null")
		require.False(info.ScheduledUntil.Valid, "scheduled until should be null")
	})

	s.Run("AfterEnqueue", func() {
		// Enqueue some tasks
		configs := []EnqueueConfig{
			{
				Kind:       "zap",
				NPending:   16,
				NRetry:     8,
				NSuccess:   4,
				NFailed:    1,
				NScheduled: 3,
				NCancelled: 2,
			},
			{
				Kind:       "foo",
				NPending:   8,
				NRetry:     1,
				NSuccess:   122,
				NFailed:    0,
				NScheduled: 8,
				NCancelled: 1,
			},
			{
				Kind:       "bar",
				NPending:   28,
				NRetry:     1,
				NSuccess:   90,
				NFailed:    2,
				NScheduled: 2,
				NCancelled: 0,
			},
		}

		var awaiting, completed int64
		for _, config := range configs {
			err := config.Enqueue(ctx, s.Broker)
			require.NoError(err, "unable to enqueue tasks")

			awaiting += int64(config.NPending) + int64(config.NRetry) + int64(config.NScheduled)
			completed += int64(config.NSuccess) + int64(config.NFailed) + int64(config.NCancelled)
		}

		info, err := s.Broker.QueueStatus(ctx)
		require.NoError(err, "unable to get queue status")
		require.NotNil(info)

		require.Len(info.Statuses, 6, "statuses should have all seven states")
		require.Len(info.Kinds, 3, "kinds should have all three kinds")
		require.Equal(awaiting, info.Awaiting, "awaiting should be %d", awaiting)
		require.Equal(completed, info.Completed, "completed should be %d", completed)
		require.True(info.Earliest.Valid, "earliest should not be null")
		require.True(info.Latest.Valid, "latest should not be null")
		require.True(info.ScheduledUntil.Valid, "scheduled until should not be null")
	})
}

func (s *BrokerTestSuite) TestTimeSeries() {
	ctx := s.Context()
	require := s.Require()

	s.Run("Empty", func() {
		now := time.Now().Truncate(time.Minute)
		after := now.Add(-time.Hour)
		interval := 5 * time.Minute

		series, err := s.Broker.TimeSeries(ctx, after, now, interval)
		require.NoError(err, "unable to get time series")

		require.Len(series, 12, "expected 12 periods")
		for _, period := range series {
			require.True(period.Timestamp.After(after) || period.Timestamp.Equal(after), "period timestamp should be after or equal to the after time, got %s vs %s", period.Timestamp, after)
			require.True(period.Timestamp.Before(now), "period timestamp should be before the now time, got %s vs %s", period.Timestamp, now)
			require.Zero(period.Tasks, "period tasks should be 0, got %d", period.Tasks)
		}
	})
}
