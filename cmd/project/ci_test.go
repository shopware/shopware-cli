package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
)

func TestBinCICommand(t *testing.T) {
	cmdExecutor := executor.NewLocal("/project")

	version := binCICommand(t.Context(), cmdExecutor, "--version")
	assert.Equal(t, []string{"php", "bin/ci", "--version"}, version.Cmd.Args)

	assetInstall := binCICommand(t.Context(), cmdExecutor, "asset:install")
	assert.Equal(t, []string{"php", "bin/ci", "asset:install"}, assetInstall.Cmd.Args)
}

func TestRunTransparentCommandPreservesExecutorEnv(t *testing.T) {
	cmdExecutor := executor.NewLocal("/project").WithEnv(map[string]string{
		"PROJECT_ROOT": "/project",
		"ADMIN_ROOT":   "/project/vendor/shopware/administration",
	})

	proc := cmdExecutor.NPMCommand(t.Context(), "run", "dev")

	applyTransparentEnv(proc)

	assert.Contains(t, proc.Cmd.Env, "PROJECT_ROOT=/project",
		"executor-provided PROJECT_ROOT must survive a transparent command")
	assert.Contains(t, proc.Cmd.Env, "ADMIN_ROOT=/project/vendor/shopware/administration",
		"executor-provided ADMIN_ROOT must survive a transparent command")
	assert.Contains(t, proc.Cmd.Env, "LOCK_DSN=flock",
		"transparent command defaults must still be applied")
}

func TestRunTransparentCommandFallsBackToProcessEnv(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TRANSPARENT_ENV_MARKER", "present")

	proc := &executor.Process{Cmd: exec.CommandContext(t.Context(), "true")}
	require.Nil(t, proc.Cmd.Env)

	applyTransparentEnv(proc)

	assert.Contains(t, proc.Cmd.Env, "SHOPWARE_CLI_TRANSPARENT_ENV_MARKER=present",
		"a command without an explicit env must inherit the current process environment")
	assert.Contains(t, proc.Cmd.Env, "LOCK_DSN=flock")
}

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
