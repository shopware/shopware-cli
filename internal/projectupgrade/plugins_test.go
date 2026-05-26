package projectupgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeInstalledJSON(t *testing.T, projectDir string, packages []installedPackage) {
	t.Helper()

	installedDir := filepath.Join(projectDir, "vendor", "composer")
	require.NoError(t, os.MkdirAll(installedDir, 0o755))

	data, err := json.MarshalIndent(installedJSON{Packages: packages}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(installedDir, "installed.json"), data, 0o644))
}

func TestRemoveIncompatiblePluginsRemovesCustomPluginsThatDontSatisfyTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Incompatible"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Compatible"), 0o755))

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":     "6.5.8.0",
			"vendor/incompat":   "*",
			"vendor/compat":     "*",
			"unrelated/package": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []installedPackage{
		{
			Name:        "vendor/incompat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Incompatible",
			Require: map[string]string{
				"shopware/core": "~6.5.0",
			},
		},
		{
			Name:        "vendor/compat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Compatible",
			Require: map[string]string{
				"shopware/core": "^6.5",
			},
		},
		{
			Name:        "vendor/composer-installed",
			Type:        composerPluginType,
			InstallPath: "../vendor/installed",
			Require: map[string]string{
				"shopware/core": "~6.5.0",
			},
		},
	})

	removed, err := RemoveIncompatiblePlugins(composerJsonPath, "6.6.4.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor/incompat"}, removed)

	out := readJSON(t, composerJsonPath)
	requireMap := out["require"].(map[string]any)
	_, stillThere := requireMap["vendor/incompat"]
	assert.False(t, stillThere, "incompatible plugin should be removed from composer.json")
	assert.Contains(t, requireMap, "vendor/compat", "compatible plugin must remain")
	assert.Contains(t, requireMap, "unrelated/package", "unrelated package must remain")
}

func TestRemoveIncompatiblePluginsNoInstalledJSONReturnsNil(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")
	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core": "6.5.8.0",
		},
	})

	removed, err := RemoveIncompatiblePlugins(composerJsonPath, "6.6.4.0")
	require.NoError(t, err)
	assert.Empty(t, removed)
}
