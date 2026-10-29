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

func TestProjectFixNoArgsUsesResolvedProjectOutsideGit(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	cwd := t.TempDir()
	t.Chdir(cwd)

	dir := t.TempDir()
	t.Setenv("PROJECT_ROOT", dir)

	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), filepath.Join(dir, "composer.json"))
}

func TestProjectFixAcceptsNonGitDirectory(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	dir := t.TempDir()
	projectFixCmd.SetContext(t.Context())
	err := projectFixCmd.RunE(projectFixCmd, []string{dir})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), filepath.Join(dir, "composer.json"))
}
