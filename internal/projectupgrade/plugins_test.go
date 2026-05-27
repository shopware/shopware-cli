package projectupgrade

import (
	"context"
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

// fakeRegistry is a test double for Registry that returns whatever the test
// configures.
type fakeRegistry struct {
	versions map[string][]packagist.ComposerPackageVersion
	err      error
}

func (f *fakeRegistry) GetPackageVersions(_ context.Context, name string) ([]packagist.ComposerPackageVersion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.versions[name], nil
}

func TestResolveIncompatiblePluginsRemovesWhenNoRegistry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Incompatible"), 0o755))

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":     "6.5.8.0",
			"vendor/incompat":   "*",
			"unrelated/package": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/incompat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Incompatible",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	result, err := ResolveIncompatiblePlugins(t.Context(), composerJsonPath, "6.6.4.0", nil)
	require.NoError(t, err)
	require.Len(t, result.Removed(), 1)
	assert.Empty(t, result.Bumped())
	assert.Equal(t, "vendor/incompat", result.Removed()[0].Name)

	out := readJSON(t, composerJsonPath)
	requireMap := out["require"].(map[string]any)
	_, stillThere := requireMap["vendor/incompat"]
	assert.False(t, stillThere)
}

func TestResolveIncompatiblePluginsBumpsConstraintWhenRegistryHasCompatibleVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Incompatible"), 0o755))

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"vendor/incompat": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/incompat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Incompatible",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	registry := &fakeRegistry{
		versions: map[string][]packagist.ComposerPackageVersion{
			"vendor/incompat": {
				{Version: "1.0.0", Require: map[string]string{"shopware/core": "~6.5.0"}},
				{Version: "2.0.0", Require: map[string]string{"shopware/core": "^6.5 | ^6.6"}},
				{Version: "2.1.0", Require: map[string]string{"shopware/core": "^6.6"}},
				{Version: "3.0.0-rc1", Require: map[string]string{"shopware/core": "^6.6"}}, // skipped: prerelease
			},
		},
	}

	result, err := ResolveIncompatiblePlugins(t.Context(), composerJsonPath, "6.6.4.0", registry)
	require.NoError(t, err)
	require.Len(t, result.Bumped(), 1)
	assert.Empty(t, result.Removed())

	bumped := result.Bumped()[0]
	assert.Equal(t, "vendor/incompat", bumped.Name)
	assert.Equal(t, "^1.0", bumped.OldConstraint)
	assert.Equal(t, "2.1.0", bumped.NewVersion)
	assert.Equal(t, "^2.1.0", bumped.NewConstraint)

	out := readJSON(t, composerJsonPath)
	requireMap := out["require"].(map[string]any)
	assert.Equal(t, "^2.1.0", requireMap["vendor/incompat"])
}

func TestResolveIncompatiblePluginsRemovesWhenNoCompatibleRelease(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Incompatible"), 0o755))

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"vendor/incompat": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/incompat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Incompatible",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	// Only old versions, none compatible with 6.6.4.0.
	registry := &fakeRegistry{
		versions: map[string][]packagist.ComposerPackageVersion{
			"vendor/incompat": {
				{Version: "1.0.0", Require: map[string]string{"shopware/core": "~6.5.0"}},
				{Version: "1.1.0", Require: map[string]string{"shopware/core": "~6.5.0"}},
			},
		},
	}

	result, err := ResolveIncompatiblePlugins(t.Context(), composerJsonPath, "6.6.4.0", registry)
	require.NoError(t, err)
	assert.Empty(t, result.Bumped())
	require.Len(t, result.Removed(), 1)
	assert.Equal(t, "vendor/incompat", result.Removed()[0].Name)
	assert.Equal(t, "no compatible release found", result.Removed()[0].Reason)
}

func TestResolveIncompatiblePluginsRegistryErrorFallsBackToRemove(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "Incompatible"), 0o755))

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"vendor/incompat": "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{
			Name:        "vendor/incompat",
			Type:        composerPluginType,
			InstallPath: "../../custom/plugins/Incompatible",
			Require:     map[string]string{"shopware/core": "~6.5.0"},
		},
	})

	registry := &fakeRegistry{err: assertErr("network down")}
	result, err := ResolveIncompatiblePlugins(t.Context(), composerJsonPath, "6.6.4.0", registry)
	require.NoError(t, err)
	require.Len(t, result.Removed(), 1)
	assert.Contains(t, result.Removed()[0].Reason, "network down")
}

func TestResolveIncompatiblePluginsNoInstalledJSONReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")
	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core": "6.5.8.0",
		},
	})

	result, err := ResolveIncompatiblePlugins(t.Context(), composerJsonPath, "6.6.4.0", nil)
	require.NoError(t, err)
	assert.Empty(t, result.Actions)
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
