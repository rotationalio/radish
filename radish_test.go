package radish_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish"
)

func TestRadish(t *testing.T) {
	conf := mockConfig(t)

	turnip, err := radish.New(conf)
	require.NoError(t, err)

	require.NoError(t, radish.Register(turnip, new(SleepWorker)))
	require.NoError(t, radish.Register(turnip, new(SortWorker)))
	require.NoError(t, radish.Register(turnip, new(RandomFailureWorker)))
}

func TestGracefulShutdown(t *testing.T) {
	t.Skip("not implemented yet")
	// TODO: test that radish waits for all executors to finish before returning from shutdown.
}
