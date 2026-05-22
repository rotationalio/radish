package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/radish/broker/sqlite"
	"go.rtnl.ai/x/dsn"
)

func TestConnectClose(t *testing.T) {
	uri := &dsn.DSN{
		Provider: dsn.SQLite3,
		Path:     filepath.Join(os.TempDir(), "radish-test.db"),
	}
	db, err := Connect(uri)
	require.NoError(t, err, "could not connect to database")
	require.NoError(t, db.Close(), "could not close database")
}

func TestSQLiteDSN(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "sqlite:///test.db",
			expected: "file:test.db?_txlock=immediate",
		},
		{
			input:    "sqlite:///test.db?_txlock=deferred",
			expected: "file:test.db?_txlock=deferred",
		},
		{
			input:    "sqlite:///test.db?_txlock=immediate&foo=bar",
			expected: "file:test.db?_txlock=immediate&foo=bar",
		},
	}

	for _, tc := range testCases {
		uri, err := dsn.Parse(tc.input)
		require.NoError(t, err)
		require.Equal(t, tc.expected, SQLiteDSN(uri))
	}
}
