package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.rtnl.ai/radish/broker/postgres"
	"go.rtnl.ai/radish/broker/tests"
	"go.rtnl.ai/x/dsn"
)

type PostgresBrokerSuite struct {
	tests.BrokerTestSuite
	db *postgres.Broker
}

func (s *PostgresBrokerSuite) Reset() {
	_, err := s.db.Exec(context.Background(), "TRUNCATE TABLE radish_tasks RESTART IDENTITY")
	require.NoError(s.T(), err, "could not clean up test database before test")
}

func (s *PostgresBrokerSuite) SetupTest() {
	s.Reset()
}

func (s *PostgresBrokerSuite) TearDownSuite() {
	s.db.Close()
}

func TestPostgresBroker(t *testing.T) {
	var databaseURL string
	if databaseURL = os.Getenv("DATABASE_URL"); databaseURL == "" {
		t.Skip("DATABASE_URL is not set, skipping test")
		return
	}

	uri, err := dsn.Parse(databaseURL)
	require.NoError(t, err)

	db, err := postgres.Connect(uri)
	require.NoError(t, err)

	s := &PostgresBrokerSuite{
		BrokerTestSuite: tests.BrokerTestSuite{Broker: db},
		db:              db,
	}

	suite.Run(t, s)
}
