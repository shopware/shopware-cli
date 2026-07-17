//go:build darwin

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
	assert.Contains(t, guidance, "/etc/resolver/shopware.local")
	assert.Contains(t, guidance, "nameserver 127.0.0.1")
	assert.Contains(t, guidance, "port 53535")
	assert.Contains(t, guidance, "shopware-cli project proxy verify")
}
