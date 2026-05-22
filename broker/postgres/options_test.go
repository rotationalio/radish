package postgres_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish/broker/postgres"
	"go.rtnl.ai/x/dsn"
)

func TestConnectionOptions(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
			options  *postgres.Options
		}{
			{
				input:    "postgres://username:password@localhost:5432/database",
				expected: "postgres://username:password@localhost:5432/database?connect_timeout=60&fallback_application_name=radish&sslmode=prefer",
				options: &postgres.Options{
					MaxIdleConns:    128,
					MaxOpenConns:    512,
					ConnMaxLifetime: 60 * time.Minute,
					ConnMaxIdleTime: 6 * time.Minute,
				},
			},
			{
				input:    "postgres://localhost:5432/database?sslmode=prefer&connect_timeout=120&fallback_application_name=foo",
				expected: "postgres://localhost:5432/database?connect_timeout=120&fallback_application_name=foo&sslmode=prefer",
				options: &postgres.Options{
					MaxIdleConns:    128,
					MaxOpenConns:    512,
					ConnMaxLifetime: 60 * time.Minute,
					ConnMaxIdleTime: 6 * time.Minute,
				},
			},
			{
				input:    "postgres://username:password@localhost:5432/database?max_idle_conns=8&max_open_conns=32&conn_max_lifetime=120m&conn_max_idle_time=60m",
				expected: "postgres://username:password@localhost:5432/database?connect_timeout=60&fallback_application_name=radish&sslmode=prefer",
				options: &postgres.Options{
					MaxIdleConns:    8,
					MaxOpenConns:    32,
					ConnMaxLifetime: 120 * time.Minute,
					ConnMaxIdleTime: 60 * time.Minute,
				},
			},
		}

		for _, tc := range testCases {
			uri, err := dsn.Parse(tc.input)
			require.NoError(t, err, "failed to parse input dsn %q", tc.input)

			connStr, opts, err := postgres.ConnectionOptions(uri)
			require.NoError(t, err, "failed to get connection options for input dsn %q", tc.input)
			require.Equal(t, tc.expected, connStr)
			require.Equal(t, tc.options, opts)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		testCases := []struct {
			input string
			err   string
		}{
			{
				input: "postgres://username:password@localhost:5432/database?max_idle_conns=invalid",
				err:   "could not parse max idle conns: strconv.Atoi: parsing \"invalid\": invalid syntax",
			},
			{
				input: "postgres://username:password@localhost:5432/database?max_open_conns=invalid",
				err:   "could not parse max open conns: strconv.Atoi: parsing \"invalid\": invalid syntax",
			},
			{
				input: "postgres://username:password@localhost:5432/database?conn_max_lifetime=invalid",
				err:   "could not parse conn max lifetime: time: invalid duration \"invalid\"",
			},
			{
				input: "postgres://username:password@localhost:5432/database?conn_max_idle_time=invalid",
				err:   "could not parse conn max idle time: time: invalid duration \"invalid\"",
			},
		}

		for _, tc := range testCases {
			uri, err := dsn.Parse(tc.input)
			require.NoError(t, err, "failed to parse input dsn %q", tc.input)

			_, _, err = postgres.ConnectionOptions(uri)
			require.EqualError(t, err, tc.err, "expected connection string error for input dsn %q", tc.input)
		}
	})
}
