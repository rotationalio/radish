package mock_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker/mock"
	"go.rtnl.ai/radish/broker/tests"
)

type MockBrokerSuite struct {
	tests.BrokerTestSuite
	mock   *mock.Broker
	simple *mock.Simple
}

func (s *MockBrokerSuite) Reset() {
	s.mock.Reset()
	s.simple.Reset()

	s.mock.OnClose = s.simple.Close
	s.mock.OnList = s.simple.List
	s.mock.OnInfo = s.simple.Info
	s.mock.OnEnqueue = s.simple.Enqueue
	s.mock.OnSchedule = s.simple.Schedule
	s.mock.OnDequeue = s.simple.Dequeue
	s.mock.OnCancel = s.simple.Cancel
	s.mock.OnFail = s.simple.Fail
	s.mock.OnRetry = s.simple.Retry
	s.mock.OnSuccess = s.simple.Success
	s.mock.OnVacuum = s.simple.Vacuum
	s.mock.OnQueueSize = s.simple.QueueSize
}

func (s *MockBrokerSuite) SetupTest() {
	s.Reset()
}

func (s *MockBrokerSuite) SetupSubTest() {
	s.Reset()
}

func (s *MockBrokerSuite) TearDownSuite() {
	s.Reset()
}

func TestBroker(t *testing.T) {
	broker, err := mock.Connect(nil)
	require.NoError(t, err)

	s := &MockBrokerSuite{
		BrokerTestSuite: tests.BrokerTestSuite{Broker: broker},
		mock:            broker,
		simple:          &mock.Simple{},
	}

	suite.Run(t, s)
}

func (s *MockBrokerSuite) TestListEmpty() {
	s.T().Skip("not implemented for mock")
}

func (s *MockBrokerSuite) TestList() {
	s.T().Skip("not implemented for mock")
}
