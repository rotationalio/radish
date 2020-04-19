package radish_test

import (
	"testing"
	. "github.com/kansaslabs/radish"
	"github.com/stretchr/testify/require"
)

func TestRadishWorkers(t *testing.T) {
	radish, err := New(&Config{Workers: 4})
	require.NoError(t, err)
	require.Equal(t, 4, radish.NumWorkers())

	require.NoError(t, radish.SetWorkers(10))
	require.Equal(t, 10, radish.NumWorkers())

	require.NoError(t, radish.SetWorkers(3))
	require.Equal(t, 3, radish.NumWorkers())

	require.NoError(t, radish.AddWorkers(2))
	require.Equal(t, 5, radish.NumWorkers())

	require.NoError(t, radish.RemoveWorkers(1))
	require.Equal(t, 4, radish.NumWorkers())

	require.NoError(t, radish.RemoveWorkers(0))
	require.Equal(t, 4, radish.NumWorkers())
	require.NoError(t, radish.AddWorkers(0))
	require.Equal(t, 4, radish.NumWorkers())
}