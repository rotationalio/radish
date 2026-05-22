package tests

import (
	"context"
	"time"

	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker"
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

func (s *BrokerTestSuite) TestEnqueueDequeueSingleSuccessfulTask() {
	ctx := context.Background()
	require := s.Require()
	requireTask := RequireTaskFactory(require)

	// Enqueue a task
	id, err := s.Broker.Enqueue(ctx, testKind, []byte(testPayload))
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
	id, err := s.Broker.Enqueue(ctx, testKind, []byte(testPayload))
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
