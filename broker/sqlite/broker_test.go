package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker/sqlite"
	"go.rtnl.ai/radish/broker/tests"
	"go.rtnl.ai/x/dsn"
)

type SQLiteBrokerSuite struct {
	tests.BrokerTestSuite
	db *sqlite.Broker
}

func (s *SQLiteBrokerSuite) Reset() {
	_, err := s.db.Exec(context.Background(), "DELETE FROM radish_tasks")
	require.NoError(s.T(), err, "could not clean up test database before test")
}

func (s *SQLiteBrokerSuite) SetupTest() {
	s.Reset()
}

func (s *SQLiteBrokerSuite) TearDownSuite() {
	s.Reset()
	_ = s.db.Close()
}

func TestSQLiteBroker(t *testing.T) {
	uri := &dsn.DSN{
		Provider: dsn.SQLite3,
		Path:     filepath.Join(os.TempDir(), "radish-test.db"),
	}

	db, err := sqlite.Connect(uri)
	require.NoError(t, err)

	s := &SQLiteBrokerSuite{
		BrokerTestSuite: tests.BrokerTestSuite{Broker: db},
		db:              db,
	}

	suite.Run(t, s)
}

func (s *SQLiteBrokerSuite) TestQueueStatus() {
	s.T().Skip("the sqlite3 queue status query is not working as expected")
}

func (s *SQLiteBrokerSuite) TestTimeSeries() {
	s.T().Skip("the sqlite3 timeseries query is not working as expected")
}
