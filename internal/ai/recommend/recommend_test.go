package recommend

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldShowAndMarkShown(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	assert.True(t, ShouldShow("claude-code"), "an unseen client should show")

	require.NoError(t, MarkShown("claude-code"))
	assert.False(t, ShouldShow("claude-code"), "a just-shown client is suppressed")

	// A different client is independent.
	assert.True(t, ShouldShow("codex"))
}

func TestShowsAgainAfterInterval(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	b, err := json.Marshal(store{ShownAt: map[string]time.Time{"claude-code": time.Now().Add(-25 * time.Hour)}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file(), b, 0o644))

	assert.True(t, ShouldShow("claude-code"), "older than the interval should show again")
}
