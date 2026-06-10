package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

// copyServicesXMLFixture copies testdata/services-xml/<name> into a temporary
// directory, since the fixer modifies the files in place.
func copyServicesXMLFixture(t *testing.T, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "services-xml", name))))

	return tmpDir
}

func expectedServicesYAML(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", "services-xml", "expected-services.yaml"))
	require.NoError(t, err)

	return string(content)
}

func TestServicesXMLConverterIsRegistered(t *testing.T) {
	_, err := GetTools().Only("services-xml")
	assert.NoError(t, err)
}

func TestServicesXMLConverterFixesPlugin(t *testing.T) {
	tmpDir := copyServicesXMLFixture(t, "plugin")
	configDir := filepath.Join(tmpDir, "src", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(configDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedServicesYAML(t), string(content))
}

func TestServicesXMLConverterFixesExtraBundles(t *testing.T) {
	tmpDir := copyServicesXMLFixture(t, "plugin-extra-bundle")
	bundleConfigDir := filepath.Join(tmpDir, "src", "Foo", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleConfigDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(bundleConfigDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedServicesYAML(t), string(content))
}

func TestServicesXMLConverterSkipsApps(t *testing.T) {
	tmpDir := copyServicesXMLFixture(t, "app")
	configDir := filepath.Join(tmpDir, "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestServicesXMLConverterKeepsUnconvertibleFile(t *testing.T) {
	tmpDir := copyServicesXMLFixture(t, "unconvertible")
	configDir := filepath.Join(tmpDir, "src", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestServicesXMLConverterFixesProjectPlugins(t *testing.T) {
	tmpDir := copyServicesXMLFixture(t, "project")
	configDir := filepath.Join(tmpDir, "custom", "plugins", "TestPlugin", "src", "Resources", "config")

	err := ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(configDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedServicesYAML(t), string(content))
}
