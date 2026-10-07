package recommend

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/state"
)

// setupSuggest isolates the global state, project root and suppression cache in
// temp dirs for the duration of a test.
func setupSuggest(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)            // macOS UserConfigDir base
	t.Setenv("XDG_CONFIG_HOME", home) // Linux UserConfigDir base
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
}

func TestSuggestNoAgent(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "")

	client, msg := Suggest()
	assert.Empty(t, client)
	assert.Empty(t, msg)
}

func TestSuggestRecommends(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")

	client, msg := Suggest()
	assert.Equal(t, "claude-code", client)
	assert.Contains(t, msg, "ai add shopware-cli --agent claude-code")
}

func TestSuggestSkipsWhenInstalled(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	require.NoError(t, state.Save(state.File{Installed: []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal},
	}}))

	_, msg := Suggest()
	assert.Empty(t, msg, "already installed for this client")
}

func TestSuggestSkipsWhenRecentlyShown(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	require.NoError(t, MarkShown("claude-code"))

	_, msg := Suggest()
	assert.Empty(t, msg, "within the suppression window")
}

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
