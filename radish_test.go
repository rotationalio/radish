package radish_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish"
)

func TestRadish(t *testing.T) {
	turnip, err := radish.New()
	require.NoError(t, err)

	require.NoError(t, radish.Register(turnip, new(SleepWorker)))
	require.NoError(t, radish.Register(turnip, new(SortWorker)))
	require.NoError(t, radish.Register(turnip, new(RandomFailureWorker)))
}
