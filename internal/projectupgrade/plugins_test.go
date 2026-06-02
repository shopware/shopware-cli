package projectupgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func writeInstalledJSON(t *testing.T, projectDir string, packages []packagist.InstalledPackage) {
	t.Helper()

	installedDir := filepath.Join(projectDir, "vendor", "composer")
	require.NoError(t, os.MkdirAll(installedDir, 0o755))

	data, err := json.MarshalIndent(packagist.InstalledJson{Packages: packages}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(installedDir, "installed.json"), data, 0o644))
}

func TestFindNonComposerPluginsReportsUntrackedDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "TrackedPlugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "UntrackedPlugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "AnotherUntracked"), 0o755))

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/tracked",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/TrackedPlugin",
		},
	})

	orphans, err := FindNonComposerPlugins(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"AnotherUntracked", "UntrackedPlugin"}, orphans)
}

func TestFindNonComposerPluginsNoCustomPluginsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	orphans, err := FindNonComposerPlugins(dir)
	require.NoError(t, err)
	assert.Empty(t, orphans)
}

func TestFindNonComposerPluginsAllTracked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "A"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "B"), 0o755))

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{Name: "vendor/a", Type: composerPluginType, InstallPath: "../../custom/plugins/A"},
		{Name: "vendor/b", Type: composerPluginType, InstallPath: "../../custom/plugins/B"},
	})

	orphans, err := FindNonComposerPlugins(dir)
	require.NoError(t, err)
	assert.Empty(t, orphans)
}
