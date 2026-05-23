package backoff_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/radish/backoff"
)

const numAttempts = 16

type backoffAssert func(t *testing.T, attempt int, delay time.Duration)

func BackoffTest(backoff BackOff, assert backoffAssert) func(t *testing.T) {
	return func(t *testing.T) {
		for attempt := 0; attempt < numAttempts; attempt++ {
			delay := backoff.Delay(attempt)
			t.Logf("attempt %2d:  delay %s", attempt, delay)
			assert(t, attempt, delay)
		}
	}
}

func TestZeroBackoff(t *testing.T) {
	backoff, err := New(Config{Policy: PolicyZero})
	require.NoError(t, err)

	t.Run("Delay", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.Zero(t, delay, "expected delay to always be zero valued")
	}))

	backoff, err = New(Config{Policy: PolicyZero, Jitter: true, Sigma: 8 * time.Second})
	require.NoError(t, err)

	t.Run("Jitter", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.Zero(t, delay, "expected delay to always be zero valued")
	}))
}

func TestConstantBackoff(t *testing.T) {
	backoff, err := New(Config{Policy: PolicyConstant, Delay: 5 * time.Second})
	require.NoError(t, err)

	t.Run("Delay", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.Equal(t, 5*time.Second, delay, "expected delay to be constant at 5 seconds")
	}))

	backoff, err = New(Config{Policy: PolicyConstant, Delay: 5 * time.Second, Jitter: true, Sigma: 500 * time.Millisecond})
	require.NoError(t, err)
	prev := time.Duration(0)

	t.Run("Jitter", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.GreaterOrEqual(t, delay, 0*time.Second, "all delays should be positive")
		require.Less(t, delay, 10*time.Second, "all delays should be less than 10 seconds")
		require.NotEqual(t, prev, delay, "all delays should be different")
		prev = delay
	}))
}

func TestLinearBackoff(t *testing.T) {
	backoff, err := New(Config{Policy: PolicyLinear, Delay: 5 * time.Second})
	require.NoError(t, err)

	t.Run("Delay", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.GreaterOrEqual(t, delay, 0*time.Second, "all delays should be positive")
		require.Equal(t, time.Duration(attempt)*5*time.Second, delay, "expected delay to be linear")
	}))

	backoff, err = New(Config{Policy: PolicyLinear, Delay: 5 * time.Second, Jitter: true, Sigma: 500 * time.Millisecond})
	require.NoError(t, err)
	prev := time.Duration(0)

	t.Run("Jitter", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.GreaterOrEqual(t, delay, 0*time.Second, "all delays should be positive")

		mean := time.Duration(attempt) * 5 * time.Second
		require.InDelta(t, mean, delay, float64(2*time.Second), "expected delay to be close to the mean")

		require.NotEqual(t, prev, delay, "all delays should be different")
		prev = delay
	}))
}

func TestExponentialBackoff(t *testing.T) {
	backoff, err := New(Config{Policy: PolicyExponential, Delay: 1 * time.Second, Factor: 2.0})
	require.NoError(t, err)

	t.Run("Delay", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.GreaterOrEqual(t, delay, 0*time.Second, "all delays should be positive")
		mean := time.Duration(math.Pow(2.0, float64(attempt))) * 1 * time.Second
		require.Equal(t, mean, delay, "expected delay to be close to the mean")
	}))

	backoff, err = New(Config{Policy: PolicyExponential, Delay: 1 * time.Second, Factor: 2.0, Jitter: true, Sigma: 500 * time.Millisecond})
	require.NoError(t, err)
	prev := time.Duration(0)

	t.Run("Jitter", BackoffTest(backoff, func(t *testing.T, attempt int, delay time.Duration) {
		require.GreaterOrEqual(t, delay, 0*time.Second, "all delays should be positive")
		mean := time.Duration(math.Pow(2.0, float64(attempt))) * 1 * time.Second
		require.InDelta(t, mean, delay, float64(2*time.Second), "expected delay to be close to the mean")
		require.NotEqual(t, prev, delay, "all delays should be different")
		prev = delay
	}))
}
