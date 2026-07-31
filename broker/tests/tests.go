package tests

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker"
	"go.rtnl.ai/radish/broker/cursor"
	"go.rtnl.ai/radish/broker/errors"
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
}

func New(b broker.Broker) *BrokerTestSuite {
	return &BrokerTestSuite{
		Broker: b,
	}
}

func (s *BrokerTestSuite) TestListEmpty() {
	ctx := context.Background()
	require := s.Require()

	tasks, err := s.Broker.List(ctx, nil)
	require.NoError(err, "unable to list tasks")
	defer tasks.Close()

	require.Zero(tasks.Count(), "expected 0 tasks but got %d", tasks.Count())

	meta, err := tasks.List()
	require.NoError(err, "unable to list tasks")
	require.Empty(meta, "expected no tasks but got %d", len(meta))
}

func (s *BrokerTestSuite) TestList() {
	// Before this test starts enqueue tasks with several kinds and statuses.
	ctx := context.Background()
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
		defer tasks.Close()

		require.Equal(total, tasks.Count(), "expected %d tasks but got %d", total, tasks.Count())
	})

	s.Run("Filtered", func() {
		tasks, err := s.Broker.List(ctx, cursor.Where().Kinds("test").States(status.Pending))
		require.NoError(err, "unable to list tasks")
		defer tasks.Close()

		require.Equal(int64(16), tasks.Count(), "expected %d tasks but got %d", 16, tasks.Count())
	})
}

func (s *BrokerTestSuite) TestDequeueNotFound() {
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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

type EnqueueConfig struct {
	Kind       string    // the kind to enqueue
	NPending   int       // the number of tasks to enqueue
	NRetry     int       // the number of tasks to mark as retryable
	NSuccess   int       // the number of tasks to mark as succeeded
	NFailed    int       // the number of tasks to mark as failed
	NScheduled int       // the number of tasks to schedule
	After      time.Time // the time to schedule the task after
	NCancelled int       // the number of scheduled tasks to mark as cancelled

	// The tasks that were enqueued
	tasks []int64
}

func (c *EnqueueConfig) Total() int64 {
	return int64(len(c.tasks))
}

func (c *EnqueueConfig) Enqueue(ctx context.Context, b broker.Broker) (err error) {
	p, r, s, f, sc, ca := c.NPending, c.NRetry, c.NSuccess, c.NFailed, c.NScheduled, c.NCancelled
	for {
		if p+r+s+f+sc+ca == 0 {
			break
		}

		if p > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			p--
		}

		if r > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Retry(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}}, 0); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			r--
		}

		if s > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Success(ctx, id); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			s--
		}

		if f > 0 {
			var id int64
			if id, err = b.Enqueue(ctx, c.Kind, []byte(testPayload), nil); err != nil {
				return err
			}

			if err = b.Fail(ctx, id, models.AttemptErrors{{Attempt: 1, Error: "test", Timestamp: time.Now()}}); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			f--
		}

		if sc > 0 {
			var id int64
			if id, err = b.Schedule(ctx, c.Kind, []byte(testPayload), c.After.Add(randDelay()), nil); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			sc--
		}

		if ca > 0 {
			var id int64
			if id, err = b.Schedule(ctx, c.Kind, []byte(testPayload), c.After.Add(randDelay()), nil); err != nil {
				return err
			}

			if err = b.Cancel(ctx, id); err != nil {
				return err
			}

			c.tasks = append(c.tasks, id)
			ca--
		}
	}

	return nil
}

func randDelay() time.Duration {
	return time.Duration(rand.IntN(10000)) * time.Millisecond
}
