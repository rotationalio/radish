package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	query, err := Query("schema")
	require.NoError(t, err)
	require.NotEmpty(t, query)
	require.True(t, strings.HasPrefix(query, "BEGIN;"))
	require.True(t, strings.HasSuffix(query, "COMMIT;"))
}

func TestQueryInvalid(t *testing.T) {
	query, err := Query("invalid")
	require.Error(t, err)
	require.Empty(t, query)
}
