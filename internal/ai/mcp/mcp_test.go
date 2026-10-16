package mcp

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readServers reads the agent's server map back from its config file.
func readServers(t *testing.T, agent string) map[string]any {
	t.Helper()
	path, err := ConfigPath(agent)
	require.NoError(t, err)

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	servers, _ := doc[agents[agent].key].(map[string]any)

	return servers
}

func TestAddWritesDirectHTTPEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	require.NoError(t, Add("claude-code", "meteor", "https://meteor.shopware.com/mcp", "http"))

	servers := readServers(t, "claude-code")
	entry, ok := servers["meteor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "http", entry["type"])
	assert.Equal(t, "https://meteor.shopware.com/mcp", entry["url"])
}

func TestAddUsesServersKeyForVSCode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	require.NoError(t, Add("vscode", "meteor", "https://meteor.shopware.com/mcp", "http"))

	path, _ := ConfigPath("vscode")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"servers"`)
	assert.NotContains(t, string(b), `"mcpServers"`)
}

func TestAddUsesNpxBridgeForClaudeDesktop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	require.NoError(t, Add("claude-desktop", "meteor", "https://meteor.shopware.com/mcp", "http"))

	servers := readServers(t, "claude-desktop")
	entry := servers["meteor"].(map[string]any)
	assert.Equal(t, "npx", entry["command"])
	assert.Equal(t, []any{"mcp-remote", "https://meteor.shopware.com/mcp"}, entry["args"])
}

func TestAddPreservesOtherContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, _ := ConfigPath("claude-code")
	require.NoError(t, os.WriteFile(path, []byte(`{"projects":{"x":1},"mcpServers":{"other":{"type":"http","url":"https://other/mcp"}}}`), 0o644))

	require.NoError(t, Add("claude-code", "meteor", "https://meteor.shopware.com/mcp", "http"))

	b, _ := os.ReadFile(path)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	assert.Contains(t, doc, "projects", "unrelated keys are preserved")
	servers := doc["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "other", "other servers are preserved")
	assert.Contains(t, servers, "meteor")
}

func TestRemoveDeletesExactNameOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, Add("claude-code", "meteor", "https://a/mcp", "http"))
	require.NoError(t, Add("claude-code", "meteor-foo", "https://b/mcp", "http"))

	removed, err := Remove("claude-code", "meteor")
	require.NoError(t, err)
	assert.True(t, removed)

	servers := readServers(t, "claude-code")
	assert.NotContains(t, servers, "meteor")
	assert.Contains(t, servers, "meteor-foo", "a different server with a matching substring is kept")
}

func TestRemoveReportsFalseWhenAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	removed, err := Remove("claude-code", "meteor")
	require.NoError(t, err)
	assert.False(t, removed, "nothing to remove when the server is not configured")
}

func TestListReadsAcrossAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, Add("claude-code", "meteor", "https://meteor.shopware.com/mcp", "http"))
	require.NoError(t, Add("cursor", "meteor", "https://meteor.shopware.com/mcp", "http"))

	got, err := List()
	require.NoError(t, err)

	byAgent := map[string]Installed{}
	for _, i := range got {
		byAgent[i.Agent] = i
	}
	assert.Equal(t, "https://meteor.shopware.com/mcp", byAgent["Claude Code"].Source)
	assert.Equal(t, "meteor", byAgent["Cursor"].Name)
}

func TestSupported(t *testing.T) {
	assert.True(t, Supported("claude-code"))
	assert.False(t, Supported("codex"))
	assert.False(t, Supported("*"))
	assert.Equal(t, []string{"claude-code", "claude-desktop", "cursor", "vscode"}, SupportedAgents())
}
