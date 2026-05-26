package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonMarshal(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.CommandContext(t.Context(), "git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s", args, string(out))
	}
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "seed"), []byte("seed"), 0o644))
	gitCmd(t, dir, "add", "seed")
	gitCmd(t, dir, "commit", "-m", "seed", "--no-verify", "--no-gpg-sign")
	return dir
}

func TestEnsureCleanGitTreeSkipsNonRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	assert.NoError(t, ensureCleanGitTree(t.Context(), dir, false))
}

func TestEnsureCleanGitTreeAllowsCleanRepo(t *testing.T) {
	t.Parallel()
	dir := setupTestRepo(t)
	assert.NoError(t, ensureCleanGitTree(t.Context(), dir, false))
}

func TestEnsureCleanGitTreeRejectsDirtyRepo(t *testing.T) {
	t.Parallel()
	dir := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked"), []byte("x"), 0o644))

	err := ensureCleanGitTree(t.Context(), dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "working tree must be clean")
	assert.Contains(t, err.Error(), "untracked")
}

func TestEnsureCleanGitTreeAllowDirtyFlagBypassesCheck(t *testing.T) {
	t.Parallel()
	dir := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked"), []byte("x"), 0o644))

	assert.NoError(t, ensureCleanGitTree(t.Context(), dir, true))
}

func writeInstalledJSON(t *testing.T, projectDir string, packages []map[string]any) {
	t.Helper()
	installedDir := filepath.Join(projectDir, "vendor", "composer")
	require.NoError(t, os.MkdirAll(installedDir, 0o755))
	body, err := jsonMarshal(map[string]any{"packages": packages})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(installedDir, "installed.json"), body, 0o644))
}

func TestEnsureAllPluginsAreComposerManagedAllowsTrackedDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Tracked"), 0o755))
	writeInstalledJSON(t, dir, []map[string]any{
		{
			"name":         "vendor/tracked",
			"type":         "shopware-platform-plugin",
			"install-path": "../../custom/plugins/Tracked",
		},
	})

	assert.NoError(t, ensureAllPluginsAreComposerManaged(dir, false))
}

func TestEnsureAllPluginsAreComposerManagedRejectsOrphanedDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Orphan"), 0o755))

	err := ensureAllPluginsAreComposerManaged(dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not tracked by composer")
	assert.Contains(t, err.Error(), "Orphan")
	assert.Contains(t, err.Error(), "autofix composer-plugins")
}

func TestEnsureAllPluginsAreComposerManagedAllowFlagBypasses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Orphan"), 0o755))

	assert.NoError(t, ensureAllPluginsAreComposerManaged(dir, true))
}
