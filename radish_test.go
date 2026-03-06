package radish_test

import (
	"testing"

	"go.rtnl.ai/radish"
	"go.rtnl.ai/x/assert"
)

func TestVersion(t *testing.T) {
	assert.Equal(t, "0.1.0", radish.Version(true))
	assert.Equal(t, "0.1.0-alpha.1", radish.Version(false))
}
