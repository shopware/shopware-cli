package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestProjectBuiltinValidatesOnlyIncludedLocalExtensions(t *testing.T) {
	stubShopwareVersions(t)
	p := testhelper.NewProject(t).
		File("composer.json", testProjectComposerJSON.String()).
		File(".shopware-project.yml", "validation:\n  ignore_extensions:\n    - name: IgnoredPlugin\n")
	for _, name := range []string{"LocalPlugin", "IgnoredPlugin"} {
		p.CustomPlugin(name, testhelper.PluginComposer("test/"+name, "1.0.0", name+`\`+name))
		pluginDir := filepath.Join(p.Root, "custom", "plugins", name)
		writeDeprecatedServicesXML(t, pluginDir)
		testhelper.WriteFile(t, filepath.Join(pluginDir, ".DS_Store"), "store")
	}
	// Vendor symlinks are excluded; a sibling with a similar name is local code.
	for _, dir := range []string{"vendor", "vendor-local"} {
		name := "VendorPlugin"
		if dir == "vendor-local" {
			name = "SiblingPlugin"
		}
		pluginDir := filepath.Join(p.Root, dir, "test", name)
		testhelper.WriteFile(t, filepath.Join(pluginDir, "composer.json"), testhelper.PluginComposer("test/"+name, "1.0.0", name+`\`+name).String())
		writeDeprecatedServicesXML(t, pluginDir)
		require.NoError(t, os.Symlink(pluginDir, filepath.Join(p.Root, "custom", "plugins", name)))
	}

	cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
	require.NoError(t, err)
	assert.True(t, cfg.InputWasDirectory)
	require.Len(t, cfg.Extensions, 2)

	check := NewCheck()
	check.SetSourceRoot(p.Root)
	require.NoError(t, Builtin{}.Check(t.Context(), check, *cfg))
	var xmlPaths []string
	for _, result := range check.GetResults() {
		assert.NotEqual(t, "zip.disallowed_file", result.Identifier)
		if result.Identifier == "config.services_xml.deprecated" {
			xmlPaths = append(xmlPaths, result.Path)
		}
	}
	assert.ElementsMatch(t, []string{
		"custom/plugins/LocalPlugin/src/Resources/config/services.xml",
		"vendor-local/test/SiblingPlugin/src/Resources/config/services.xml",
	}, xmlPaths)
}

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

	cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
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

	cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
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

	cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
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

func TestProjectPHPVersion(t *testing.T) {
	composerWith := func(platform, require string) rootComposerJson {
		var c rootComposerJson
		c.Config.Platform.PHP = platform
		c.Require = map[string]string{"php": require}
		return c
	}

	cases := []struct {
		name     string
		cfg      *shop.Config
		composer rootComposerJson
		expected string
	}{
		{"validation php version wins", &shop.Config{PHPVersion: "8.4", Validation: &shop.ConfigValidation{PhpVersion: "8.2"}}, composerWith("8.3.0", "^8.2"), "8.2"},
		{"project config wins", &shop.Config{PHPVersion: "8.4", Docker: &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: "8.3"}}}, composerWith("8.2.0", "^8.2"), "8.4"},
		{"docker php version", &shop.Config{Docker: &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: "8.3"}}}, composerWith("8.2.0", "^8.2"), "8.3"},
		{"composer platform", &shop.Config{}, composerWith("8.3.12", "^8.2"), "8.3.12"},
		{"composer require", &shop.Config{}, composerWith("", ">=8.3"), "8.3"},
		{"invalid composer require", &shop.Config{}, composerWith("", "not a constraint"), ""},
		{"nothing set", &shop.Config{}, rootComposerJson{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, projectPHPVersion(tc.cfg, tc.composer))
		})
	}
}

func TestGetConfigFromProjectPHPVersion(t *testing.T) {
	stubShopwareVersions(t)
	p := testhelper.NewProject(t).
		File("composer.json", testProjectComposerJSON.String()).
		File(".shopware-project.yml", "php_version: \"8.4\"\n")

	cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
	require.NoError(t, err)
	assert.Equal(t, "8.4", cfg.PHPVersion)
}
