package verifier

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/testhelper"
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

var testProjectComposerJSON = testhelper.ComposerJSON{
	Type:    "project",
	Require: map[string]string{"shopware/core": "~6.6.0"},
}

func TestGetConfigFromProjectYAMLBundles(t *testing.T) {
	stubShopwareVersions(t)
	p := testhelper.NewProject(t).
		File("composer.json", testProjectComposerJSON.String()).
		File(".shopware-project.yml", testProjectYAMLSingleBundle)

	// Create bundle directory with an admin subfolder
	p.Dir("src/MyBundle/Resources/app/administration")
	adminPath := filepath.Join(p.Root, "src", "MyBundle", "Resources", "app", "administration")

	cfg, err := GetConfigFromProject(p.Root, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(p.Root, "src", "MyBundle"))
	assert.Contains(t, cfg.AdminDirectories, adminPath)
}

func TestGetConfigFromProjectYAMLBundleStorefront(t *testing.T) {
	stubShopwareVersions(t)
	p := testhelper.NewProject(t).
		File("composer.json", testProjectComposerJSON.String()).
		File(".shopware-project.yml", testProjectYAMLSingleBundle)

	// Create bundle directory with a storefront subfolder only
	p.Dir("src/MyBundle/Resources/app/storefront")
	storefrontPath := filepath.Join(p.Root, "src", "MyBundle", "Resources", "app", "storefront")

	cfg, err := GetConfigFromProject(p.Root, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(p.Root, "src", "MyBundle"))
	assert.Contains(t, cfg.StorefrontDirectories, storefrontPath)
}

func TestGetConfigFromProjectYAMLBundleDeduplication(t *testing.T) {
	stubShopwareVersions(t)

	// composer.json declares the same bundle as the YAML config
	bundleComposer := testProjectComposerJSON
	bundleComposer.Extra = map[string]any{
		"shopware-bundles": map[string]any{"src/MyBundle": map[string]string{"name": "MyBundle"}},
	}
	p := testhelper.NewProject(t).
		File("composer.json", bundleComposer.String()).
		File(".shopware-project.yml", testProjectYAMLSingleBundle)

	p.Dir("src/MyBundle")

	cfg, err := GetConfigFromProject(p.Root, true)
	assert.NoError(t, err)

	bundleSrcPath := filepath.Join(p.Root, "src", "MyBundle")
	count := 0
	for _, d := range cfg.SourceDirectories {
		if d == bundleSrcPath {
			count++
		}
	}
	assert.Equal(t, 1, count, "bundle declared in both composer.json and YAML config should only appear once in SourceDirectories")
}
