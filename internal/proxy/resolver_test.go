package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManualSetupInstructions(t *testing.T) {
	t.Parallel()

	t.Run("includes resolver step and trust command", func(t *testing.T) {
		t.Parallel()
		got := ManualSetupInstructions("shopware.local", "/tmp/rootCA.pem", true)

		assert.Contains(t, got, "Point your system's DNS at the proxy:")
		assert.Contains(t, got, "shopware.local")
		assert.Contains(t, got, "Trust the local HTTPS certificate:")
		assert.Contains(t, got, "/tmp/rootCA.pem")
		assert.Contains(t, got, "proxy verify")
	})

	t.Run("omits the trust step when includeTrust is false", func(t *testing.T) {
		t.Parallel()
		got := ManualSetupInstructions("shopware.local", "/tmp/rootCA.pem", false)

		assert.NotContains(t, got, "Trust the local HTTPS certificate:")
		assert.NotContains(t, got, "/tmp/rootCA.pem")
		assert.Contains(t, got, "proxy verify")
	})

	t.Run("indents the resolver detail under its heading", func(t *testing.T) {
		t.Parallel()
		got := ManualSetupInstructions("shopware.local", "/tmp/rootCA.pem", true)

		// Every non-empty resolver-detail line is indented beneath the heading.
		lines := strings.Split(got, "\n")
		assert.Contains(t, lines[0], "Point your system's DNS")
		assert.True(t, strings.HasPrefix(lines[1], "  "), "resolver detail should be indented")
	})
}
