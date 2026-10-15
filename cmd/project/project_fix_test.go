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

// TestProjectFixNoArgsOutsideProjectReturnsError checks that automatic project
// discovery fails outside a Shopware project.
func TestProjectFixNoArgsOutsideProjectReturnsError(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	chdirOutsideProject(t)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find Shopware project")
}

// TestProjectFixNoArgsAppliesGitGuardToResolvedProject checks the discovered
// project rather than the working directory used to invoke the command.
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

// TestProjectFixRejectsNonGitDirectory checks that an explicit path outside a
// working tree is rejected before project loading.
func TestProjectFixRejectsNonGitDirectory(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
	assert.Contains(t, err.Error(), dir)
}

// TestProjectFixPassesGitGuardWithGitWorkingTree verifies that a real working
// tree reaches project loading even when composer.json is missing.
func TestProjectFixPassesGitGuardWithGitWorkingTree(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	initFixRepository(t, dir)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "composer.json")
}

// initFixRepository creates a real working tree in dir without inheriting
// repository locations that could redirect Git elsewhere.
func initFixRepository(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", "init", dir)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "GIT_DIR=") || strings.HasPrefix(entry, "GIT_WORK_TREE=")
	})
	require.NoError(t, cmd.Run())
}
