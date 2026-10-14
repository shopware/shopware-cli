package recommend

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/skills"
)

// setupSuggest isolates the project root and suppression cache in temp dirs and
// stubs the skills.sh lookup to "nothing installed" by default.
func setupSuggest(t *testing.T) {
	t.Helper()
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	prev := listSkills
	listSkills = func(_ context.Context, _ string, _ bool) ([]skills.Installed, error) {
		return nil, nil
	}
	t.Cleanup(func() { listSkills = prev })
}

func TestSuggestNoAgent(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "")

	client, msg := Suggest(t.Context())
	assert.Empty(t, client)
	assert.Empty(t, msg)
}

func TestSuggestRecommends(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")

	client, msg := Suggest(t.Context())
	assert.Equal(t, "claude-code", client)
	assert.Contains(t, msg, "ai add shopware-cli --agent claude-code --global")
	assert.Contains(t, msg, "ask the user", "describe the install, do not instruct it")
}

func TestSuggestSkipsWhenInstalled(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	// skills.sh reports the guidance skill installed for Claude Code (global).
	listSkills = func(_ context.Context, _ string, global bool) ([]skills.Installed, error) {
		if global {
			return []skills.Installed{{Name: "shopware-cli", Agents: []string{"Claude Code"}}}, nil
		}
		return nil, nil
	}

	_, msg := Suggest(t.Context())
	assert.Empty(t, msg, "already installed for this client")
}

func TestSuggestSkipsWhenRecentlyShown(t *testing.T) {
	setupSuggest(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	require.NoError(t, MarkShown("claude-code"))

	_, msg := Suggest(t.Context())
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

func TestMatchesAgent(t *testing.T) {
	assert.True(t, matchesAgent([]string{"Claude Code"}, "claude-code"))
	assert.False(t, matchesAgent([]string{"Claude Code"}, "cursor"))
}
