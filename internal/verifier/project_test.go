package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/validation"
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

	toolResult := runCheckTool(t, Builtin{}, *cfg)
	require.Empty(t, toolResult.Errors)
	var xmlPaths []string
	for _, result := range toolResult.Findings {
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

func TestGetConfigFromProjectPHPVersion(t *testing.T) {
	stubShopwareVersions(t)

	for _, tc := range []struct {
		name     string
		config   string
		expected string
	}{
		{"unset", "{}", ""},
		{"project target", "php_version: '8.4'", "8.4"},
		{"validation target", "validation:\n  php_version: '8.2'", "8.2"},
		{"validation override", "php_version: '8.4'\nvalidation:\n  php_version: '8.2'", "8.2"},
		{"empty validation target", "php_version: '8.4'\nvalidation:\n  php_version: ''", "8.4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testhelper.NewProject(t).
				File("composer.json", testProjectComposerJSON.String()).
				File(".config/shopware-project.yml", tc.config)

			cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cfg.PHPVersion)
		})
	}
}

func TestProjectBuiltinPHPVersion(t *testing.T) {
	stubShopwareVersions(t)

	for _, tc := range []struct {
		name             string
		extensionVersion string
		wantSyntaxError  bool
	}{
		{name: "project target"},
		{name: "extension override", extensionVersion: "7.4", wantSyntaxError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plugin := testhelper.PluginComposer("test/plugin", "1.0.0", `TestPlugin\TestPlugin`)
			plugin.Require["shopware/core"] = ">=6.4"
			p := testhelper.NewProject(t).
				File("composer.json", testProjectComposerJSON.String()).
				File(".config/shopware-project.yml", "php_version: '8.2'").
				CustomPlugin("TestPlugin", plugin).
				File("custom/plugins/TestPlugin/src/Status.php", "<?php\nenum Status { case Active; }\n")
			if tc.extensionVersion != "" {
				p.File("custom/plugins/TestPlugin/.config/shopware-extension.yml", "validation:\n  php_version: '"+tc.extensionVersion+"'")
			}

			cfg, err := GetConfigFromProject(t.Context(), p.Root, true)
			require.NoError(t, err)
			require.Len(t, cfg.Extensions, 1)

			toolResult := runCheckTool(t, Builtin{}, *cfg)
			require.Empty(t, toolResult.Errors)
			var lintResults []validation.CheckResult
			for _, result := range toolResult.Findings {
				if result.Identifier == "php.linter" {
					lintResults = append(lintResults, result)
					assert.Equal(t, validation.SeverityError, result.Severity)
					assert.Equal(t, "custom/plugins/TestPlugin/src/Status.php", result.Path)
				}
			}
			assert.Equal(t, tc.wantSyntaxError, len(lintResults) > 0)
			assert.Equal(t, tc.extensionVersion, cfg.Extensions[0].GetExtensionConfig().Validation.PhpVersion)
		})
	}
}
