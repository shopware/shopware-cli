package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
