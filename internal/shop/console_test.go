package shop

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
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
		dir := writeCommandCache(t, "console_commands.json", `{"commands":[{"name":"about","description":"Display information about the current project"}]}`)

		resp, err := ReadCachedConsoleCompletion(dir)
		require.NoError(t, err)
		require.Len(t, resp.Commands, 1)
		assert.Equal(t, "about", resp.Commands[0].Name)
		assert.Equal(t, "Display information about the current project", resp.Commands[0].Description)
	})
}

func TestReadCachedComposerCompletion(t *testing.T) {
	t.Run("missing cache", func(t *testing.T) {
		_, err := ReadCachedComposerCompletion(t.TempDir())
		require.Error(t, err)
	})

	t.Run("reads cache", func(t *testing.T) {
		dir := writeCommandCache(t, "composer_commands.json", `{"commands":[{"name":"install","description":"Installs the project dependencies"}]}`)

		resp, err := ReadCachedComposerCompletion(dir)
		require.NoError(t, err)
		require.Len(t, resp.Commands, 1)
		assert.Equal(t, "install", resp.Commands[0].Name)
		assert.Equal(t, "Installs the project dependencies", resp.Commands[0].Description)
	})
}

func TestGetConsoleCompletionLoadsWhenCacheMissing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "var", "cache"), 0o755))

	resp, err := GetConsoleCompletion(t.Context(), dir, func(ctx context.Context, args ...string) *exec.Cmd {
		assert.Equal(t, []string{"list", "--format=json"}, args)
		return exec.CommandContext(ctx, "printf", "%s", `{"commands":[{"name":"cache:clear"}]}`)
	})
	require.NoError(t, err)
	assert.True(t, resp.HasCommand("cache:clear"))

	cached, err := ReadCachedConsoleCompletion(dir)
	require.NoError(t, err)
	assert.True(t, cached.HasCommand("cache:clear"))
}

func writeCommandCache(t *testing.T, name, contents string) string {
	t.Helper()
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "var", "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, name), []byte(contents), 0o644))
	return dir
}
