package project

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectFixNoArgsOutsideProjectReturnsError(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	chdirOutsideProject(t)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find Shopware project")
}

func TestProjectFixNoArgsAppliesGitGuardToResolvedProject(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	cwd := t.TempDir()
	initFixRepository(t, cwd)
	t.Chdir(cwd)

	dir := t.TempDir()
	t.Setenv("PROJECT_ROOT", dir)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
	assert.Contains(t, err.Error(), dir)
}

func TestProjectFixRejectsNonGitDirectory(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
	assert.Contains(t, err.Error(), dir)
}

func TestProjectFixPassesGitGuardWithGitWorkingTree(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	initFixRepository(t, dir)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "composer.json")
}

func initFixRepository(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "init", dir)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "GIT_DIR=") || strings.HasPrefix(entry, "GIT_WORK_TREE=")
	})
	require.NoError(t, cmd.Run())
}
