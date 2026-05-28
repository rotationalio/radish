package radish_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/confire/contest"
	"go.rtnl.ai/radish"
	"go.rtnl.ai/radish/backoff"
)

// The values in the test environment variables should result a configuration that is
// equal to the validConfig variable when processed. Anytime a configuration value is
// added to this package, it should be added to this map and have a non-default value
// for testing and validation purposes.
var testEnv = contest.Env{
	"DATABASE_URL":           "postgres://radish:radish@localhost:5432/radish?sslmode=disable",
	"RADISH_NUM_WORKERS":     "32",
	"RADISH_TASK_RETRIES":    "5",
	"RADISH_TASK_TIMEOUT":    "120s",
	"RADISH_POLL_INTERVAL":   "20s",
	"RADISH_POLL_JITTER":     "50ms",
	"RADISH_RETENTION":       "48h",
	"RADISH_VACUUM_INTERVAL": "3h",
	"RADISH_BACKOFF_POLICY":  "exponential",
	"RADISH_BACKOFF_DELAY":   "8s",
	"RADISH_BACKOFF_FACTOR":  "1.25",
	"RADISH_BACKOFF_JITTER":  "true",
	"RADISH_BACKOFF_SIGMA":   "32ms",
}

// Used for mock testing of the config.
var mockEnv = contest.Env{
	"RADISH_MANAGED_DB":      "true",
	"RADISH_NUM_WORKERS":     "4",
	"RADISH_TASK_RETRIES":    "2",
	"RADISH_TASK_TIMEOUT":    "5s",
	"RADISH_POLL_INTERVAL":   "1s",
	"RADISH_POLL_JITTER":     "5ms",
	"RADISH_RETENTION":       "24h",
	"RADISH_VACUUM_INTERVAL": "3h",
	"RADISH_BACKOFF_POLICY":  "exponential",
	"RADISH_BACKOFF_DELAY":   "8s",
	"RADISH_BACKOFF_FACTOR":  "1.25",
	"RADISH_BACKOFF_JITTER":  "true",
	"RADISH_BACKOFF_SIGMA":   "32ms",
}

// This config should always pass validation and should match the testEnv.
// For a minimal valid config for tests, use [conftest.Config] or [conftest.Unmarked].
var validConfig = radish.Config{
	DatabaseURL:    "postgres://radish:radish@localhost:5432/radish?sslmode=disable",
	NumWorkers:     32,
	TaskRetries:    5,
	TaskTimeout:    120 * time.Second,
	PollInterval:   20 * time.Second,
	PollJitter:     50 * time.Millisecond,
	Retention:      48 * time.Hour,
	VacuumInterval: 3 * time.Hour,
	Backoff: backoff.Config{
		Policy: backoff.PolicyExponential,
		Delay:  8 * time.Second,
		Factor: 1.25,
		Jitter: true,
		Sigma:  32 * time.Millisecond,
	},
	Conn: nil,
}

func TestConfig(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		t.Cleanup(testEnv.Set())

		conf, err := radish.LoadConfig()
		require.NoError(t, err, "could not process config from environment")
		require.Equal(t, validConfig, conf, "valid config should be equal to the expected valid config")
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Cleanup(testEnv.Set())

		// Set the invalid environment variables one at a time and reset once the test is complete.
		invalid := contest.Env{
			"DATABASE_URL":         "",
			"RADISH_NUM_WORKERS":   "0",
			"RADISH_TASK_TIMEOUT":  "0s",
			"RADISH_POLL_INTERVAL": "0s",
		}

		// The expected errors for each invalid environment variable.
		errs := map[string]string{
			"DATABASE_URL":         "invalid configuration: radish.database_url either the database DSN or a connection must be provided (specify RADISH_MANAGED_DB=1)",
			"RADISH_NUM_WORKERS":   "invalid configuration: radish.num_workers the number of workers must be at least 1",
			"RADISH_TASK_TIMEOUT":  "invalid configuration: radish.timeout is required but not set",
			"RADISH_POLL_INTERVAL": "invalid configuration: radish.poll_interval is required but not set",
		}

		for key := range invalid {
			cleanup := invalid.Set(key)
			_, err := radish.LoadConfig()
			require.EqualError(t, err, errs[key], "expected error for %s environment variable", key)
			cleanup()
		}
	})
}

func mockConfig(t *testing.T) *radish.Config {
	t.Helper()

	t.Cleanup(mockEnv.Set())
	cfg, err := radish.LoadConfig()
	require.NoError(t, err)

	cfg.Conn = &sql.DB{}
	return &cfg
}
