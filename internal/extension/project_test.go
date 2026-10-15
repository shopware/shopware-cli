package extension

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestFindExtensionsFromProjectReturnsSortedPartialResult(t *testing.T) {
	project := t.TempDir()
	writePlugin(t, project, "FirstFolder", "Swag\\Zzz\\ZzzPlugin", `compatibility_date: "2026-01-01"`)
	writePlugin(t, project, "LastFolder", "Swag\\Aaa\\AaaPlugin", `compatibility_date: "2026-01-01"`)
	writePlugin(t, project, "BrokenPlugin", "Swag\\Broken\\BrokenPlugin", `
compatibility_date: "2026-01-01"
anonymize:
  unknown_field: true
`)
	testhelper.WriteFile(t, filepath.Join(project, "vendor", "swag", "broken", "composer.json"), "{ invalid json")
	testhelper.WriteFile(t, filepath.Join(project, "composer.lock"), testhelper.ComposerLock(
		testhelper.LockPackage{Name: "swag/broken", Version: "1.0.0", Type: ComposerTypePlugin},
	))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "custom", "plugins", "not-an-extension"), 0o755))

	extensions, err := FindExtensionsFromProject(t.Context(), project, false)

	var configErr *ConfigError
	var packageErr *ComposerPackageError
	require.ErrorAs(t, err, &configErr)
	require.ErrorAs(t, err, &packageErr)
	assert.Equal(t, "swag/broken", packageErr.Package)
	require.Len(t, extensions, 2)
	var names []string
	for _, ext := range extensions {
		name, err := ext.GetName()
		require.NoError(t, err)
		names = append(names, name)
	}
	assert.Equal(t, []string{"AaaPlugin", "ZzzPlugin"}, names)

	local, err := FindExtensionsFromProject(t.Context(), project, true)
	require.ErrorAs(t, err, &configErr)
	assert.NotErrorAs(t, err, &packageErr)
	assert.Len(t, local, 2)
}

func TestGetShopwareProjectConstraintComposerJson(t *testing.T) {
	testCases := []struct {
		Name       string
		Files      map[string]string
		Constraint string
		Error      string
	}{
		{
			Name: "Get constraint from composer.json",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "~6.5.0"
	}}`,
			},
			Constraint: "~6.5.0",
		},
		{
			Name: "Get constraint from composer.lock",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{
		"packages": [
{
"name": "shopware/core",
"version": "6.5.0"
}
]}`,
			},
			Constraint: "6.5.*",
		},
		{
			Name: "Branch installed, determine by Kernel.php",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{
		"packages": [
{
"name": "shopware/core",
"version": "dev-trunk"
}
]}`,
				"src/Core/composer.json": `{}`,
				"src/Core/Kernel.php": `<?php
final public const SHOPWARE_FALLBACK_VERSION = '6.6.9999999.9999999-dev';
`,
			},
			Constraint: "6.5.*",
		},
		{
			Name: "Get constraint from kernel (shopware/shopware case)",
			Files: map[string]string{
				"composer.json":          `{}`,
				"src/Core/composer.json": `{}`,
				"src/Core/Kernel.php": `<?php
final public const SHOPWARE_FALLBACK_VERSION = '6.6.9999999.9999999-dev';
`,
			},
			Constraint: "~6.6.0",
		},

		// error cases
		{
			Name:  "no composer.json",
			Files: map[string]string{},
			Error: "could not read composer.json",
		},

		{
			Name: "composer.json broken",
			Files: map[string]string{
				"composer.json": `broken`,
			},
			Error: "could not parse composer.json",
		},

		{
			Name: "composer.json with no shopware package",
			Files: map[string]string{
				"composer.json": `{}`,
			},
			Error: "missing shopware/core requirement in composer.json",
		},

		{
			Name: "composer.json malformed version, without lock, so we cannot fall down",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
			},
			Constraint: "6.5.*",
		},

		{
			Name: "composer.json malformed version, lock does not contain shopware/core",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{"packages": []}`,
			},
			Constraint: "6.5.*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for file, content := range tc.Files {
				testhelper.WriteFile(t, filepath.Join(tmpDir, file), content)
			}

			constraint, err := GetShopwareProjectConstraint(tmpDir)

			if tc.Constraint == "" {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.Error)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tc.Constraint, constraint.String())
		})
	}
}

func TestFindAssetSourcesOfProjectYAMLBundles(t *testing.T) {
	// Minimal composer.json without extra bundles
	tmpDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "~6.6.0"},
	})

	// Create the bundle directory
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), 0o755))

	shopCfg := &shop.Config{
		Build: &shop.ConfigBuild{
			Bundles: []shop.ConfigProjectBundle{
				{Path: "src/MyBundle"},
			},
		},
	}

	sources := FindAssetSourcesOfProject(t.Context(), tmpDir, shopCfg)

	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "MyBundle")

	for _, s := range sources {
		if s.Name == "MyBundle" {
			assert.Equal(t, path.Join(tmpDir, "src", "MyBundle"), s.Path)
		}
	}
}

func TestFindAssetSourcesOfProjectYAMLBundleNameOverride(t *testing.T) {
	tmpDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "~6.6.0"},
	})
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), 0o755))

	shopCfg := &shop.Config{
		Build: &shop.ConfigBuild{
			Bundles: []shop.ConfigProjectBundle{
				{Path: "src/MyBundle", Name: "CustomBundleName"},
			},
		},
	}

	sources := FindAssetSourcesOfProject(t.Context(), tmpDir, shopCfg)

	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "CustomBundleName")
	assert.NotContains(t, names, "MyBundle")
}

func TestFindAssetSourcesOfProjectYAMLBundleDeduplication(t *testing.T) {
	// composer.json declares the same bundle path
	tmpDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "~6.6.0"},
		Extra:   map[string]any{"shopware-bundles": map[string]any{"src/MyBundle": map[string]string{"name": "MyBundle"}}},
	})
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), 0o755))

	shopCfg := &shop.Config{
		Build: &shop.ConfigBuild{
			Bundles: []shop.ConfigProjectBundle{
				{Path: "src/MyBundle"},
			},
		},
	}

	sources := FindAssetSourcesOfProject(t.Context(), tmpDir, shopCfg)

	count := 0
	for _, s := range sources {
		if s.Name == "MyBundle" {
			count++
		}
	}

	assert.Equal(t, 1, count, "bundle declared in both composer.json and YAML config should only appear once")
}

func TestFindAssetSourcesOfProjectWithoutComposerJSON(t *testing.T) {
	sources := FindAssetSourcesOfProject(t.Context(), t.TempDir(), &shop.Config{})

	assert.Empty(t, sources)
}
