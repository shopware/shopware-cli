package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWSLResolverGuidance(t *testing.T) {
	t.Parallel()

	g := WSLResolverGuidance()

	// The three fix commands, in order, with resolve as the last nsswitch source.
	assert.Contains(t, g, "sudo apt-get install -y libnss-resolve")
	assert.Contains(t, g, "hosts: files dns resolve")
	// The reason it must be last (no internet upstream on WSL).
	assert.Contains(t, g, "after")
	// A way to confirm internet DNS still works.
	assert.Contains(t, g, "getent hosts one.one.one.one")
	// It names nsswitch, the file being changed.
	assert.Contains(t, g, "nsswitch")
}
