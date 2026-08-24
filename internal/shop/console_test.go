package shop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsoleResponseHasCommand(t *testing.T) {
	var resp ConsoleResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"commands": [
			{"name": "cache:clear"},
			{"name": "hidden:cmd", "hidden": true}
		]
	}`), &resp))

	assert.True(t, resp.HasCommand("cache:clear"))
	assert.False(t, resp.HasCommand("hidden:cmd"))
	assert.False(t, resp.HasCommand("missing"))
}

func TestReadCachedConsoleCompletion(t *testing.T) {
	t.Run("missing cache", func(t *testing.T) {
		_, err := ReadCachedConsoleCompletion(t.TempDir())
		require.Error(t, err)
	})

	t.Run("reads cache", func(t *testing.T) {
		dir := t.TempDir()
		cacheDir := filepath.Join(dir, "var", "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "console_commands.json"), []byte(`{"commands":[{"name":"about"}]}`), 0o644))

		resp, err := ReadCachedConsoleCompletion(dir)
		require.NoError(t, err)
		require.Len(t, resp.Commands, 1)
		assert.Equal(t, "about", resp.Commands[0].Name)
	})
}
