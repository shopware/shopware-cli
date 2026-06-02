package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectCISafetyCheck(t *testing.T) {
	t.Run("allows dirty git working tree in CI", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(map[string]string{"CI": "true"}))

		assert.NoError(t, err)
	})

	t.Run("allows dirty git working tree with force", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, true, mapGetenv(nil))

		assert.NoError(t, err)
	})

	t.Run("rejects dirty git working tree outside CI without force", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(nil))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "project ci removes source files")
		assert.Contains(t, err.Error(), "--force")
	})

	t.Run("allows clean git working tree outside CI without force", func(t *testing.T) {
		root := newCleanGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(nil))

		assert.NoError(t, err)
	})
}

func newDirtyGitRepository(t *testing.T) string {
	t.Helper()

	root := newCleanGitRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("local work"), 0o644))

	return root
}

func newCleanGitRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	runGit(t, root, "init")

	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, output)
}

func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}
