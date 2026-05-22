package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubShopwareVersions replaces the network-backed version lookup with a
// fixed list, so GetConfigFromProject does not hit repo.packagist.org.
func stubShopwareVersions(t *testing.T) {
	t.Helper()
	original := getShopwareVersions
	t.Cleanup(func() { getShopwareVersions = original })
	getShopwareVersions = func(context.Context) ([]string, error) {
		return []string{"6.6.0.0"}, nil
	}
}

const testProjectYAMLSingleBundle = `compatibility_date: "2024-01-01"
build:
  bundles:
    - path: src/MyBundle
`

func TestGetConfigFromProjectYAMLBundles(t *testing.T) {
	stubShopwareVersions(t)
	tmpDir := t.TempDir()

	// Minimal composer.json with shopware/core requirement
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"}
	}`), 0o644))

	// Create bundle directory with an admin subfolder
	adminPath := filepath.Join(tmpDir, "src", "MyBundle", "Resources", "app", "administration")
	assert.NoError(t, os.MkdirAll(adminPath, 0o755))

	// Write .shopware-project.yml with the bundle declared
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(tmpDir, "src", "MyBundle"))
	assert.Contains(t, cfg.AdminDirectories, adminPath)
}

func TestGetConfigFromProjectYAMLBundleStorefront(t *testing.T) {
	stubShopwareVersions(t)
	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"}
	}`), 0o644))

	// Create bundle directory with a storefront subfolder only
	storefrontPath := filepath.Join(tmpDir, "src", "MyBundle", "Resources", "app", "storefront")
	assert.NoError(t, os.MkdirAll(storefrontPath, 0o755))

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(tmpDir, "src", "MyBundle"))
	assert.Contains(t, cfg.StorefrontDirectories, storefrontPath)
}

func TestGetConfigFromProjectYAMLBundleDeduplication(t *testing.T) {
	stubShopwareVersions(t)
	tmpDir := t.TempDir()

	// composer.json declares the same bundle
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-bundles": {"src/MyBundle": {"name": "MyBundle"}}}
	}`), 0o644))

	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), 0o755))

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	bundleSrcPath := filepath.Join(tmpDir, "src", "MyBundle")
	count := 0
	for _, d := range cfg.SourceDirectories {
		if d == bundleSrcPath {
			count++
		}
	}
	assert.Equal(t, 1, count, "bundle declared in both composer.json and YAML config should only appear once in SourceDirectories")
}
