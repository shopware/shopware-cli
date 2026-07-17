package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrustBlockedGuidance(t *testing.T) {
	t.Parallel()

	guidance := TrustBlockedGuidance("/state/rootCA.pem")

	// The three options: admin session, IT hand-off with the exact command,
	// and the no-admin path.
	assert.Contains(t, guidance, "administrator rights")
	assert.Contains(t, guidance, "shopware-cli project proxy setup")
	assert.Contains(t, guidance, "IT team")
	assert.Contains(t, guidance, TrustInstructions("/state/rootCA.pem"))
	assert.Contains(t, guidance, "security warning")
	assert.Contains(t, guidance, "Firefox")
	assert.Contains(t, guidance, "/state/rootCA.pem")
}
