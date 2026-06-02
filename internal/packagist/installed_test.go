package packagist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeInstalled(t *testing.T, projectRoot string, installed InstalledJson) {
	t.Helper()
	dir := filepath.Join(projectRoot, "vendor", "composer")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.MarshalIndent(installed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), data, 0o644))
}

func TestReadInstalledJsonMissingReturnsEmpty(t *testing.T) {
	t.Parallel()
	installed, err := ReadInstalledJson(t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, installed)
	assert.Empty(t, installed.Packages)
}

func TestReadInstalledJsonParsesPackages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeInstalled(t, dir, InstalledJson{Packages: []InstalledPackage{
		{Name: "vendor/a", Type: "shopware-platform-plugin", InstallPath: "../../custom/plugins/A"},
	}})

	installed, err := ReadInstalledJson(dir)
	require.NoError(t, err)
	require.NotNil(t, installed)
	require.Len(t, installed.Packages, 1)
	assert.Equal(t, "vendor/a", installed.Packages[0].Name)
}

func TestReadInstalledJsonMalformedReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	composerDir := filepath.Join(dir, "vendor", "composer")
	require.NoError(t, os.MkdirAll(composerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(composerDir, "installed.json"), []byte("{not json"), 0o644))

	_, err := ReadInstalledJson(dir)
	require.Error(t, err)
}

func TestInstallDirNameDirectChild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "custom", "plugins")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "MyPlugin"), 0o755))

	pkg := InstalledPackage{InstallPath: "../../custom/plugins/MyPlugin"}
	name, ok := pkg.InstallDirName(dir, base)
	assert.True(t, ok)
	assert.Equal(t, "MyPlugin", name)
}

func TestInstallDirNameNotUnderBase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "custom", "plugins")

	pkg := InstalledPackage{InstallPath: "../vendor/installed"}
	_, ok := pkg.InstallDirName(dir, base)
	assert.False(t, ok)
}

func TestInstallDirNameNestedIsNotDirectChild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, "custom", "plugins")

	pkg := InstalledPackage{InstallPath: "../../custom/plugins/Group/Nested"}
	_, ok := pkg.InstallDirName(dir, base)
	assert.False(t, ok)
}

func TestInstallDirNameEmptyPath(t *testing.T) {
	t.Parallel()
	pkg := InstalledPackage{}
	_, ok := pkg.InstallDirName("/project", "/project/custom/plugins")
	assert.False(t, ok)
}
