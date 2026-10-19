package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readServers reads the project's .mcp.json server map back.
func readServers(t *testing.T, dir string) map[string]any {
	t.Helper()

	b, err := os.ReadFile(ConfigPath(dir))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	servers, _ := doc[serversKey].(map[string]any)

	return servers
}

func TestAddWritesStandardHTTPEntry(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))

	servers := readServers(t, dir)
	entry, ok := servers["meteor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "http", entry["type"])
	assert.Equal(t, "https://meteor.shopware.com/mcp", entry["url"])
}

func TestAddWritesToMcpJSONInProjectRoot(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))

	assert.FileExists(t, filepath.Join(dir, ".mcp.json"))
}

func TestAddDefaultsTransportToHTTP(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", ""))

	entry := readServers(t, dir)["meteor"].(map[string]any)
	assert.Equal(t, "http", entry["type"])
}

func TestAddPreservesOtherContent(t *testing.T) {
	dir := t.TempDir()
	path := ConfigPath(dir)
	require.NoError(t, os.WriteFile(path, []byte(`{"someKey":1,"mcpServers":{"other":{"type":"http","url":"https://other/mcp"}}}`), 0o644))

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))

	b, _ := os.ReadFile(path)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	assert.Contains(t, doc, "someKey", "unrelated keys are preserved")
	servers := doc["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "other", "other servers are preserved")
	assert.Contains(t, servers, "meteor")
}

func TestRemoveDeletesExactNameOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Add(dir, "meteor", "https://a/mcp", "http"))
	require.NoError(t, Add(dir, "meteor-foo", "https://b/mcp", "http"))

	removed, err := Remove(dir, "meteor")
	require.NoError(t, err)
	assert.True(t, removed)

	servers := readServers(t, dir)
	assert.NotContains(t, servers, "meteor")
	assert.Contains(t, servers, "meteor-foo", "a different server with a matching substring is kept")
}

func TestRemoveReportsFalseWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	removed, err := Remove(dir, "meteor")
	require.NoError(t, err)
	assert.False(t, removed, "nothing to remove when the server is not configured")
}

func TestHasReportsPresenceWithoutWriting(t *testing.T) {
	dir := t.TempDir()

	has, err := Has(dir, "meteor")
	require.NoError(t, err)
	assert.False(t, has)
	_, statErr := os.Stat(ConfigPath(dir))
	assert.True(t, os.IsNotExist(statErr), "Has must not create the config file")

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))
	has, err = Has(dir, "meteor")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestListReadsServers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))
	require.NoError(t, Add(dir, "other", "https://other/mcp", "http"))

	got, err := List(dir)
	require.NoError(t, err)

	require.Len(t, got, 2)
	// List sorts by name.
	assert.Equal(t, "meteor", got[0].Name)
	assert.Equal(t, "https://meteor.shopware.com/mcp", got[0].Source)
	assert.Equal(t, "other", got[1].Name)
}

func TestListOnMissingFileIsEmpty(t *testing.T) {
	got, err := List(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestWriteUpdatesSymlinkTargetAndKeepsMode(t *testing.T) {
	dir := t.TempDir()

	// Make .mcp.json a symlink to a file elsewhere with a non-default mode.
	realPath := filepath.Join(dir, "store", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(realPath), 0o755))
	require.NoError(t, os.WriteFile(realPath, []byte("{}\n"), 0o600))

	link := ConfigPath(dir)
	require.NoError(t, os.Symlink(realPath, link))

	require.NoError(t, Add(dir, "meteor", "https://meteor.shopware.com/mcp", "http"))

	// The link is still a link, its target carries the write, and the mode is kept.
	fi, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSymlink, "the symlink is preserved, not replaced")

	b, err := os.ReadFile(realPath)
	require.NoError(t, err)
	assert.Contains(t, string(b), "meteor", "the write lands on the symlink target")

	tfi, err := os.Stat(realPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), tfi.Mode().Perm(), "the existing file mode is preserved")
}
