package scaffolding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		parts        []string
		namespace    string
		composerName string
	}{
		{
			name:         "SwagBasicExample",
			parts:        []string{"Swag", "Basic", "Example"},
			namespace:    `Swag\BasicExample`,
			composerName: "swag/basic-example",
		},
		{
			name:         "AcmePayPal",
			parts:        []string{"Acme", "Pay", "Pal"},
			namespace:    `Acme\PayPal`,
			composerName: "acme/pay-pal",
		},
		{
			name:         "Swag2Example",
			parts:        []string{"Swag2", "Example"},
			namespace:    `Swag2\Example`,
			composerName: "swag2/example",
		},
		{
			name:         "",
			parts:        nil,
			namespace:    "",
			composerName: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.parts, splitPascalCase(test.name))
			assert.Equal(t, test.namespace, DeriveNamespace(test.name))
			assert.Equal(t, test.composerName, DeriveComposerName(test.name))
		})
	}
}

func TestCreateScaffoldingFiles(t *testing.T) {
	extensionDir := t.TempDir()

	require.NoError(t, CreateScaffoldingFiles(extensionDir, "SwagBasicExample"))

	expectedFiles := []string{
		".gitignore",
		"composer.json",
		"phpunit.xml",
		filepath.Join("src", "SwagBasicExample.php"),
		filepath.Join("src", "Resources", "config", "config.xml"),
		filepath.Join("tests", "TestBootstrap.php"),
	}
	for _, file := range expectedFiles {
		assert.FileExists(t, filepath.Join(extensionDir, file))
	}

	content, err := os.ReadFile(filepath.Join(extensionDir, "composer.json"))
	require.NoError(t, err)
	var composer struct {
		Name  string `json:"name"`
		Extra struct {
			PluginClass string `json:"shopware-plugin-class"`
		} `json:"extra"`
	}
	require.NoError(t, json.Unmarshal(content, &composer))
	assert.Equal(t, "swag/basic-example", composer.Name)
	assert.Equal(t, `Swag\BasicExample\SwagBasicExample`, composer.Extra.PluginClass)

	pluginClass, err := os.ReadFile(filepath.Join(extensionDir, "src", "SwagBasicExample.php"))
	require.NoError(t, err)
	assert.Contains(t, string(pluginClass), `namespace Swag\BasicExample;`)
	assert.Contains(t, string(pluginClass), "class SwagBasicExample extends Plugin")
}

func TestCreateExtensionDir(t *testing.T) {
	parent := t.TempDir()
	extensionDir := filepath.Join(parent, "SwagBasicExample")

	require.NoError(t, CreateExtensionDir(extensionDir))
	assert.DirExists(t, extensionDir)
	assert.ErrorContains(t, CreateExtensionDir(extensionDir), "already exists")

	filePath := filepath.Join(parent, "file")
	require.NoError(t, os.WriteFile(filePath, nil, 0o644))
	assert.ErrorContains(t, CreateExtensionDir(filePath), "not a directory")

	missingParent := filepath.Join(parent, "missing", "Extension")
	assert.ErrorContains(t, CreateExtensionDir(missingParent), "parent directory does not exist")
	assert.NoDirExists(t, filepath.Dir(missingParent))
}

func TestRemoveCreatedExtensionDir(t *testing.T) {
	for _, pluginRoot := range []string{"plugins", "static-plugins"} {
		t.Run(pluginRoot, func(t *testing.T) {
			extensionDir := filepath.Join(t.TempDir(), "custom", pluginRoot, "SwagBasicExample")
			require.NoError(t, os.MkdirAll(extensionDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(extensionDir, "file.txt"), nil, 0o644))

			require.NoError(t, RemoveCreatedExtensionDir(extensionDir))
			assert.NoDirExists(t, extensionDir)
			// Removing an already absent extension is safe and idempotent.
			require.NoError(t, RemoveCreatedExtensionDir(extensionDir))
		})
	}
}

func TestRemoveCreatedExtensionDirRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "custom", "plugins")
	require.NoError(t, os.MkdirAll(pluginRoot, 0o755))

	ordinaryDir := filepath.Join(root, "ordinary", "SwagBasicExample")
	require.NoError(t, os.MkdirAll(ordinaryDir, 0o755))
	assert.ErrorContains(t, RemoveCreatedExtensionDir(ordinaryDir), "not an extension directory")
	assert.DirExists(t, ordinaryDir)

	assert.Error(t, RemoveCreatedExtensionDir(""))
	assert.Error(t, RemoveCreatedExtensionDir(string(filepath.Separator)))
	assert.ErrorContains(t, RemoveCreatedExtensionDir(pluginRoot), "not an extension directory")
	assert.DirExists(t, pluginRoot)

	target := filepath.Join(pluginRoot, "Target")
	link := filepath.Join(pluginRoot, "Link")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.Symlink(target, link))
	assert.ErrorContains(t, RemoveCreatedExtensionDir(link), "symlink")
	assert.DirExists(t, target)
}
