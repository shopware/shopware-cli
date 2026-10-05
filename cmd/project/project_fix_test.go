package project

import (
	"os"
	"path/filepath"
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
	require.NoError(t, os.Mkdir(filepath.Join(cwd, ".git"), 0o755))
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

func TestProjectFixPassesGitGuardWithGitDirectory(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "composer.json")
}
