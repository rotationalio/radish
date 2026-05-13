package status_test

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/radish/status"
)

var (
	defaultInvalid = []any{"foo", "123", "INVALID", 257, -1, 3.14, struct{}{}, true, false}
	statusValues   = []status.Status{
		status.StatusUnknown,
		status.StatusPending,
		status.StatusRetry,
		status.StatusScheduled,
		status.StatusRunning,
		status.StatusSucceeded,
		status.StatusFailed,
		status.StatusCancelled,
	}
	statusStrings = []string{
		"unknown",
		"pending",
		"retry",
		"scheduled",
		"running",
		"succeeded",
		"failed",
		"cancelled",
	}
)

const (
	dbVarcharLimit = 16
)

func TestString(t *testing.T) {
	for i, enum := range statusValues {
		require.Equal(t, statusStrings[i], enum.String(), "expected status to have string representation %q, got %q", statusStrings[i], enum.String())
	}

	// Test Zero Values
	zero := status.Status(0)
	require.Equal(t, status.StatusUnknown.String(), zero.String(), "expected status to have string representation \"unknown\" for zero value")

	empty, err := status.Parse("")
	require.NoError(t, err, "failed to parse empty string for status")
	require.Equal(t, status.StatusUnknown.String(), empty.String(), "expected status to have string representation \"unknown\" for empty string not %q", empty.String())
}

func TestStringBounds(t *testing.T) {
	max := uint8(0)
	min := uint8(255)

	for i := range statusValues {
		if uint8(i) > max {
			max = uint8(i)
		}
		if uint8(i) < min {
			min = uint8(i)
		}
	}

	above := status.Status(max + 1)
	require.Equal(t, status.StatusUnknown.String(), above.String(), "expected status to have string representation \"unknown\" for unknown value")

	// Test zero value
	if min > 0 {
		zero := status.Status(0)
		require.Equal(t, status.StatusUnknown.String(), zero.String(), "expected status to have string representation \"unknown\" for zero value")
	}
}

func TestParse(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		makeTestCases := func(i int, enum status.Status) []any {
			tests := make([]any, 0, 8)
			tests = append(tests, statusStrings[i])
			tests = append(tests, enum.Uint8())
			tests = append(tests, float64(enum.Uint8()))
			tests = append(tests, enum)
			tests = append(tests, strings.ToUpper(statusStrings[i]), strings.ToLower(statusStrings[i]))
			tests = append(tests, mixedCase(statusStrings[i]))

			return tests
		}

		for i, enum := range statusValues {
			tests := makeTestCases(i, enum)
			for _, input := range tests {
				actual, err := status.Parse(input)
				require.NoError(t, err, "failed to parse valid status value %#v", input)
				require.Equal(t, enum, actual, "expected parsing valid status value %#v", input)
			}
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		for _, str := range defaultInvalid {
			actual, err := status.Parse(str)
			require.Error(t, err, "expected parsing invalid status value %q to error", str)
			require.Equal(t, uint8(0), actual.Uint8(), "expected parsing invalid status value %q to return zero value, got %d", str, actual.Uint8())
		}
	})
}

func TestJSON(t *testing.T) {
	t.Run("Serialization", func(t *testing.T) {
		for _, enum := range statusValues {
			orig := status.Status(enum.Uint8())
			data, err := json.Marshal(orig)
			require.NoError(t, err, "failed to marshal status value %q", orig.String())

			cmp := status.Status(0)
			err = json.Unmarshal(data, &cmp)
			require.NoError(t, err, "failed to unmarshal status value %q", orig.String())
			require.Equal(t, orig, cmp, "unmarshaled status does not match original")
		}
	})

	t.Run("Errors", func(t *testing.T) {
		inputs := make([][]byte, 0, len(defaultInvalid)+2)
		inputs = append(inputs, []byte(`"unquoted`), []byte(`{"missing":}`)) // add bad JSON inputs

		// Add parse errors
		for _, v := range defaultInvalid {
			if data, err := json.Marshal(v); err == nil {
				inputs = append(inputs, data)
			}
		}

		for _, data := range inputs {
			cmp := status.Status(0)
			err := json.Unmarshal(data, &cmp)
			require.Error(t, err, "expected unmarshaling invalid status JSON value %q to error", string(data))
			require.Equal(t, uint8(0), cmp.Uint8(), "expected unmarshaling invalid status JSON value %q to return zero value, got %d", string(data), cmp.Uint8())
		}
	})
}

func TestDatabase(t *testing.T) {
	t.Run("VARCHAR", func(t *testing.T) {
		// Ensure that all string representations are less than or equal to the db VARCHAR limit
		for _, enum := range statusValues {
			require.LessOrEqual(t, len(enum.String()), dbVarcharLimit, "expected status value %q to be less than or equal to %d characters", enum.String(), dbVarcharLimit)
		}
	})
}

func mixedCase(s string) string {
	b := make([]rune, len(s))
	for i, r := range s {
		// Flip a coin and make the character upper or lower case
		if rand.Intn(2) == 0 {
			r = unicode.ToLower(r)
		} else {
			r = unicode.ToUpper(r)
		}
		b[i] = r
	}
	return string(b)
}
