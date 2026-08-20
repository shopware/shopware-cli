package proxy

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOSResolutionHint(t *testing.T) {
	t.Parallel()

	hint := osResolutionHint("shopware.local", true)

	// States what is already known, so users debug the right layer.
	assert.Contains(t, hint, "The DNS server works, but your system is not asking it")
	// Actionable steps for the two universal causes.
	assert.Contains(t, hint, "shopware-cli project proxy setup")
	assert.Contains(t, hint, "disconnect the VPN")
	// No third-party product names.
	for _, product := range []string{"Cisco", "Umbrella", "Zscaler", "WARP"} {
		assert.NotContains(t, hint, product)
	}

	if runtime.GOOS == "darwin" {
		assert.Contains(t, hint, "Bonjour")
		assert.Contains(t, hint, "--domain shopware.internal")

		// The mDNS note only applies to .local domains.
		assert.NotContains(t, osResolutionHint("dev.internal", true), "Bonjour")
	}
}

func TestOSResolutionHintWithoutWildcardSupport(t *testing.T) {
	t.Parallel()

	hint := osResolutionHint("shopware.local", false)

	// Explains WHY automatic DNS cannot work...
	assert.Contains(t, hint, "systemd-resolved")
	assert.Contains(t, hint, "does not run it")
	// ...how to FIX the root cause...
	assert.Contains(t, hint, "sudo systemctl enable --now systemd-resolved")
	assert.Contains(t, hint, "sudo apt install systemd-resolved")
	assert.Contains(t, hint, "stub-resolv.conf")
	assert.Contains(t, hint, "proxy setup\" again")
	// ...and the manual last resort with an honest warning about the fix.
	assert.Contains(t, hint, "changes how your whole system resolves DNS")
	assert.Contains(t, hint, "127.0.0.1 <shop-name>.shopware.local")
}

func TestRandomProbeHostnameIsUniqueAndInZone(t *testing.T) {
	t.Parallel()

	a := randomProbeHostname("shopware.local")
	b := randomProbeHostname("shopware.local")

	assert.True(t, strings.HasSuffix(a, ".shopware.local"))
	assert.NotEqual(t, a, b)
}
