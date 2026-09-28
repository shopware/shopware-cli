package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgent(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	Version = "1.2.3"

	assert.Equal(t, "shopware-cli/1.2.3", UserAgent())
}
