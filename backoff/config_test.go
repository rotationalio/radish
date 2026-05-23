package backoff_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/radish/backoff"
)

func TestConfig(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		testCases := []Config{
			{
				Policy: PolicyZero,
			},
			{
				Policy: PolicyConstant,
				Delay:  8 * time.Second,
			},
			{
				Policy: PolicyConstant,
				Delay:  8 * time.Second,
				Jitter: true,
				Sigma:  32 * time.Millisecond,
			},
			{
				Policy: PolicyLinear,
				Delay:  2 * time.Second,
			},
			{
				Policy: PolicyLinear,
				Delay:  2 * time.Second,
				Jitter: true,
				Sigma:  32 * time.Millisecond,
			},
			{
				Policy: PolicyExponential,
				Delay:  2 * time.Second,
				Factor: 2.0,
			},
			{
				Policy: PolicyExponential,
				Delay:  2 * time.Second,
				Factor: 2.0,
				Jitter: true,
				Sigma:  32 * time.Millisecond,
			},
		}

		for i, tc := range testCases {
			require.NoError(t, tc.Validate(), "expected config %d to be valid", i)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		testCases := []struct {
			conf Config
			err  string
		}{
			{
				conf: Config{},
				err:  "invalid configuration: backoff.policy is required but not set",
			},
			{
				conf: Config{
					Policy: "foo",
				},
				err: "invalid configuration: backoff.policy unknown policy: foo",
			},
			{
				conf: Config{
					Policy: PolicyConstant,
					Delay:  0,
				},
				err: "invalid configuration: backoff.delay is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyConstant,
					Delay:  -8 * time.Second,
				},
				err: "invalid configuration: backoff.delay is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyConstant,
					Delay:  8 * time.Second,
					Jitter: true,
				},
				err: "invalid configuration: backoff.sigma is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyLinear,
					Delay:  0,
				},
				err: "invalid configuration: backoff.delay is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyLinear,
					Delay:  -8 * time.Second,
				},
				err: "invalid configuration: backoff.delay is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyLinear,
					Delay:  8 * time.Second,
					Jitter: true,
				},
				err: "invalid configuration: backoff.sigma is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyExponential,
					Delay:  2 * time.Second,
					Factor: 0,
				},
				err: "invalid configuration: backoff.factor is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyExponential,
					Delay:  -8 * time.Second,
					Factor: 1.25,
				},
				err: "invalid configuration: backoff.delay is required but not set",
			},
			{
				conf: Config{
					Policy: PolicyExponential,
					Delay:  2 * time.Second,
					Factor: 1.25,
					Jitter: true,
				},
				err: "invalid configuration: backoff.sigma is required but not set",
			},
		}

		for i, tc := range testCases {
			require.EqualError(t, tc.conf.Validate(), tc.err, "expected config %d to be invalid", i)
		}
	})
}
