package testhelper

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func TestPluginComposerDerivesLabelAndAutoload(t *testing.T) {
	m := decode(t, PluginComposer("swag/demo", "2.0.0", `Swag\Demo\Demo`).String())

	assert.Equal(t, "swag/demo", m["name"])
	assert.Equal(t, "shopware-platform-plugin", m["type"])
	assert.Equal(t, "2.0.0", m["version"])
	assert.Equal(t, map[string]any{"shopware/core": "~6.6.0"}, m["require"])
	assert.Equal(t, map[string]any{
		"shopware-plugin-class": `Swag\Demo\Demo`,
		"label":                 map[string]any{"en-GB": "Demo"},
	}, m["extra"])
	assert.Equal(t, map[string]any{"psr-4": map[string]any{`Swag\Demo\`: "src/"}}, m["autoload"])
}

func TestComposerJSONOmitsZeroFields(t *testing.T) {
	assert.JSONEq(t, `{}`, ComposerJSON{}.String())

	// A plugin without a version must render without the key, not with "".
	m := decode(t, PluginComposer("frosh/no-version", "", `NoVersion`).String())
	_, hasVersion := m["version"]
	assert.False(t, hasVersion)
	// A class without a namespace cannot produce a PSR-4 prefix.
	_, hasAutoload := m["autoload"]
	assert.False(t, hasAutoload)
}

func TestComposerLock(t *testing.T) {
	assert.JSONEq(t, `{"packages": [], "packages-dev": []}`, ComposerLock())

	assert.JSONEq(t, `{
		"packages": [
			{"name": "shopware/core", "version": "v6.6.10.3"},
			{"name": "swag/demo", "version": "2.0.0", "type": "shopware-platform-plugin"}
		],
		"packages-dev": []
	}`, ComposerLock(
		LockPackage{Name: "shopware/core", Version: "v6.6.10.3"},
		LockPackage{Name: "swag/demo", Version: "2.0.0", Type: "shopware-platform-plugin"},
	))

	assert.JSONEq(t, `{
		"packages": [
			{"name": "shopware/core", "version": "v6.6.10.3", "require": {"php": ">=8.2"}}
		],
		"packages-dev": []
	}`, ComposerLock(
		LockPackage{Name: "shopware/core", Version: "v6.6.10.3", Require: map[string]string{"php": ">=8.2"}},
	))
}

func TestProjectWritesRelativeFiles(t *testing.T) {
	p := NewProject(t).
		File("composer.lock", ComposerLock()).
		VendorPackage("swag/demo", PluginComposer("swag/demo", "2.0.0", `Swag\Demo\Demo`)).
		CustomPlugin("LocalPlugin", PluginComposer("acme/local-plugin", "1.0.0", `Acme\LocalPlugin\LocalPlugin`))

	assert.FileExists(t, p.Root+"/composer.lock")
	assert.FileExists(t, p.Root+"/vendor/swag/demo/composer.json")
	assert.FileExists(t, p.Root+"/custom/plugins/LocalPlugin/composer.json")
}

func TestComposerJSONExtendedFields(t *testing.T) {
	c := PluginComposer("swag/demo", "1.0.0", `Swag\Demo\Demo`)
	c.Description = "Test plugin"
	c.Authors = []string{"Test"}
	c.RequireDev = map[string]string{"shopware/deployment-helper": "^1.0"}
	c.Extra = map[string]any{"description": map[string]string{"en-GB": "Demo plugin"}}

	m := decode(t, c.String())
	assert.Equal(t, "Test plugin", m["description"])
	assert.Equal(t, []any{map[string]any{"name": "Test"}}, m["authors"])
	assert.Equal(t, map[string]any{"shopware/deployment-helper": "^1.0"}, m["require-dev"])
	extra, ok := m["extra"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `Swag\Demo\Demo`, extra["shopware-plugin-class"])
	assert.Equal(t, map[string]any{"en-GB": "Demo plugin"}, extra["description"])
}

func TestExtensionDirAndPluginHelpers(t *testing.T) {
	dir := ExtensionDir(t, ComposerJSON{Name: "swag/demo", Type: "shopware-bundle"})
	assert.FileExists(t, dir+"/composer.json")

	pluginDir := NewPlugin(t, "FroshTools")
	m := decode(t, readFile(t, pluginDir+"/composer.json"))
	assert.Equal(t, "test/frosh-tools", m["name"])
	assert.Equal(t, "shopware-platform-plugin", m["type"])
	extra, ok := m["extra"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `FroshTools\FroshTools`, extra["shopware-plugin-class"])

	appDir := NewApp(t, "MyExampleApp")
	assert.FileExists(t, appDir+"/manifest.xml")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestComposerJSONRepositoriesAndLockDist(t *testing.T) {
	m := decode(t, ComposerJSON{
		Name: "shopware/production",
		Repositories: []map[string]any{
			{"type": "path", "url": "custom/static-plugins/*", "options": map[string]any{"symlink": true}},
		},
	}.String())
	assert.Equal(t, []any{map[string]any{
		"type": "path", "url": "custom/static-plugins/*",
		"options": map[string]any{"symlink": true},
	}}, m["repositories"])

	assert.JSONEq(t, `{
		"packages": [
			{"name": "acme/custom-plugin", "version": "1.0.0", "type": "shopware-platform-plugin",
			 "dist": {"type": "path", "url": "custom/static-plugins/MyCustomPlugin"}}
		],
		"packages-dev": []
	}`, ComposerLock(LockPackage{
		Name: "acme/custom-plugin", Version: "1.0.0", Type: "shopware-platform-plugin",
		Dist: map[string]string{"type": "path", "url": "custom/static-plugins/MyCustomPlugin"},
	}))
}
