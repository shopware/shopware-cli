//go:build linux

package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolverBlockedGuidance(t *testing.T) {
	t.Parallel()

	guidance := ResolverBlockedGuidance("shopware.local")

	assert.Contains(t, guidance, "administrator rights")
	assert.Contains(t, guidance, "IT team")
	assert.Contains(t, guidance, resolvedDropInPath)
	assert.Contains(t, guidance, "DNS=127.0.0.1:53535")
	assert.Contains(t, guidance, "systemctl reload-or-restart systemd-resolved")
	assert.Contains(t, guidance, "shopware-cli project proxy verify")
}
