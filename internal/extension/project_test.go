package extension

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

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
				tmpFile := filepath.Join(tmpDir, file)
				parentDir := filepath.Dir(tmpFile)

				if _, err := os.Stat(parentDir); os.IsNotExist(err) {
					assert.NoError(t, os.MkdirAll(parentDir, os.ModePerm))
				}

				assert.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))
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
	tmpDir := t.TempDir()

	// Minimal composer.json without extra bundles
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{"require": {"shopware/core": "~6.6.0"}}`), 0o644))

	// Create the bundle directory
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), os.ModePerm))

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
	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{"require": {"shopware/core": "~6.6.0"}}`), 0o644))
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), os.ModePerm))

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
	tmpDir := t.TempDir()

	// composer.json declares the same bundle path
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-bundles": {"src/MyBundle": {"name": "MyBundle"}}}
	}`), 0o644))
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), os.ModePerm))

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
