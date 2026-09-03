package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindClosestShopwareProject(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	projectDir := createShopwareProject(t, "composer.json")
	nestedDir := filepath.Join(projectDir, "custom", "plugins", "Example")
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))
	t.Chdir(nestedDir)

	found, err := FindClosestShopwareProject(false)

	require.NoError(t, err)
	assert.Equal(t, projectDir, found)
}

func TestFindClosestShopwareProjectFromLockFile(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	projectDir := createShopwareProject(t, "composer.lock")
	t.Chdir(projectDir)

	found, err := FindClosestShopwareProject(false)

	require.NoError(t, err)
	assert.Equal(t, projectDir, found)
}

func TestFindClosestShopwareProjectUsesEnvironmentOverride(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "partly-configured-project")
	t.Setenv("PROJECT_ROOT", projectDir+string(filepath.Separator))

	found, err := FindClosestShopwareProject(false)

	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(projectDir), found)
}

func TestFindClosestShopwareProjectFallback(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	currentDir := t.TempDir()
	t.Chdir(currentDir)

	found, err := FindClosestShopwareProject(true)

	require.NoError(t, err)
	assert.Equal(t, currentDir, found)
}

func TestFindClosestShopwareProjectNotFound(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(t.TempDir())

	_, err := FindClosestShopwareProject(false)

	assert.EqualError(t, err, "cannot find Shopware project in current directory")
}

func TestFindClosestShopwareProjectRequiresConsole(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	projectDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "composer.json"),
		[]byte(`{"require":{"shopware/core":"*" }}`),
		0o644,
	))
	t.Chdir(projectDir)

	_, err := FindClosestShopwareProject(false)

	assert.ErrorContains(t, err, "cannot find Shopware project")
}

func createShopwareProject(t *testing.T, composerFile string) string {
	t.Helper()

	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "bin", "console"), nil, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, composerFile),
		[]byte(`{"require":{"shopware/core":"*" }}`),
		0o644,
	))
	return projectDir
}
