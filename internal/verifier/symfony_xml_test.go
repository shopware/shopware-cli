package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

// copySymfonyXMLFixture copies testdata/symfony-xml/<name> into a temporary
// directory, since the fixer modifies the files in place.
func copySymfonyXMLFixture(t *testing.T, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "symfony-xml", name))))

	return tmpDir
}

func expectedFixtureYAML(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", "symfony-xml", name))
	require.NoError(t, err)

	return string(content)
}

func assertConvertedConfigDir(t *testing.T, configDir string) {
	t.Helper()

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "routes.xml"))

	services, err := os.ReadFile(filepath.Join(configDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-services.yaml"), string(services))

	routes, err := os.ReadFile(filepath.Join(configDir, "routes.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-routes.yaml"), string(routes))
}

func TestSymfonyXMLConverterIsRegistered(t *testing.T) {
	_, err := GetTools().Only("symfony-xml")
	assert.NoError(t, err)
}

func TestSymfonyXMLConverterFixesPlugin(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "plugin")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = SymfonyXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assertConvertedConfigDir(t, filepath.Join(tmpDir, "src", "Resources", "config"))
}

func TestSymfonyXMLConverterFixesExtraBundles(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "plugin-extra-bundle")
	bundleConfigDir := filepath.Join(tmpDir, "src", "Foo", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = SymfonyXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleConfigDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(bundleConfigDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-services.yaml"), string(content))
}

func TestSymfonyXMLConverterSkipsApps(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "app")
	configDir := filepath.Join(tmpDir, "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = SymfonyXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.FileExists(t, filepath.Join(configDir, "routes.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
	assert.NoFileExists(t, filepath.Join(configDir, "routes.yaml"))
}

func TestSymfonyXMLConverterKeepsUnconvertibleFile(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "unconvertible")
	configDir := filepath.Join(tmpDir, "src", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = SymfonyXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestSymfonyXMLConverterFixesProjectPlugins(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "project")

	err := SymfonyXMLConverter{}.Fix(t.Context(), ToolConfig{RootDir: tmpDir})
	require.NoError(t, err)

	assertConvertedConfigDir(t, filepath.Join(tmpDir, "custom", "plugins", "TestPlugin", "src", "Resources", "config"))
}
