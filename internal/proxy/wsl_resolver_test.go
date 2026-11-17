package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWSLResolverGuidance(t *testing.T) {
	t.Parallel()

	g := WSLResolverGuidance()

	// The fix commands: install the module, then edit the hosts line in place.
	assert.Contains(t, g, "sudo apt-get install -y libnss-resolve")
	assert.Contains(t, g, "sudo sed -i -E '/^hosts:/{")
	// The existing hosts line is never deleted.
	assert.NotContains(t, g, "/^hosts:/d")
	// The reason it must be last (no internet upstream on WSL).
	assert.Contains(t, g, "after")
	// A way to confirm internet DNS still works.
	assert.Contains(t, g, "getent hosts one.one.one.one")
	// It names nsswitch, the file being changed.
	assert.Contains(t, g, "nsswitch")
}

func TestNoSystemdResolvedGuidanceOnWSL(t *testing.T) {
	t.Parallel()

	wsl := noSystemdResolvedGuidance("shopware.local", true)

	// On WSL the resolv.conf symlink would break all other DNS.
	assert.NotContains(t, wsl, "ln -sf")
	assert.Contains(t, wsl, "systemd=true")
	assert.Contains(t, wsl, "127.0.0.1 <shop-name>.shopware.local")

	// Elsewhere the symlink step stays, with an honest warning about it.
	plain := noSystemdResolvedGuidance("shopware.local", false)
	assert.Contains(t, plain, "stub-resolv.conf")
	assert.Contains(t, plain, "changes how your whole system resolves DNS")
}
