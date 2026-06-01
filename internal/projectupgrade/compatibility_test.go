package projectupgrade

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestCheckPluginCompatibilityClassifiesEachPlugin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"vendor/compat":   "^1.0",
			"vendor/updates":  "^1.0",
			"vendor/blocker":  "^1.0",
			"transitive/skip": "^1.0", // not in installed.json -> ignored
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/compat",
			Type:        composerPluginType,
			Version:     "1.2.0",
			InstallPath: "../../custom/plugins/Compat",
			Require:     map[string]string{"shopware/core": "^6.5 | ^6.6"},
		},
		{
			Name:        "vendor/updates",
			Type:        composerPluginType,
			Version:     "1.0.0",
			InstallPath: "../../custom/plugins/Updates",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
		{
			Name:        "vendor/blocker",
			Type:        composerPluginType,
			Version:     "1.0.0",
			InstallPath: "../../custom/plugins/Blocker",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	registry := &fakeRegistry{
		versions: map[string][]packagist.ComposerPackageVersion{
			"vendor/updates": {
				{Version: "2.0.0", Require: map[string]string{"shopware/core": "^6.6"}},
			},
			"vendor/blocker": {
				{Version: "1.1.0", Require: map[string]string{"shopware/core": "~6.5.0"}},
			},
		},
	}

	results, err := CheckPluginCompatibility(t.Context(), composerJsonPath, "6.6.4.0", registry)
	require.NoError(t, err)
	require.Len(t, results, 3, "transitive/skip must be ignored - it is not in installed.json")

	byName := map[string]PluginCompat{}
	for _, r := range results {
		byName[r.Name] = r
	}

	assert.Equal(t, CompatCompatible, byName["vendor/compat"].Status)
	assert.Equal(t, "1.2.0", byName["vendor/compat"].CurrentVersion)

	assert.Equal(t, CompatUpdatable, byName["vendor/updates"].Status)
	assert.Equal(t, "1.0.0", byName["vendor/updates"].CurrentVersion)
	assert.Equal(t, "2.0.0", byName["vendor/updates"].NewVersion)

	assert.Equal(t, CompatBlocker, byName["vendor/blocker"].Status)
	assert.Equal(t, "1.0.0", byName["vendor/blocker"].CurrentVersion)
}

func TestCheckPluginCompatibilityReportsUnknownOnRegistryError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":  "6.5.8.0",
			"store/mysteryx": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "store/mysteryx",
			Type:        composerPluginType,
			Version:     "1.0.0",
			InstallPath: "../store/mysteryx",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	registry := &fakeRegistry{err: assertErr("no token configured")}

	results, err := CheckPluginCompatibility(t.Context(), composerJsonPath, "6.6.4.0", registry)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, CompatUnknown, results[0].Status, "registry errors surface as 'unknown' so the user can retry with credentials")
	assert.True(t, results[0].Status.IsBlocker(), "unknown counts as a blocker - the resolver will drop the plugin")
}

func TestCheckPluginCompatibilityIgnoresNonPlatformPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"symfony/console": "^6.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "symfony/console",
			Type:        "library", // not a shopware-platform-plugin
			Version:     "6.4.0",
			InstallPath: "../symfony/console",
		},
	})

	results, err := CheckPluginCompatibility(t.Context(), composerJsonPath, "6.6.4.0", nil)
	require.NoError(t, err)
	assert.Empty(t, results, "non-platform-plugin libraries are not part of the upgrade plan")
}
