package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

const servicesXMLFixture = `<?xml version="1.0" ?>
<container xmlns="http://symfony.com/schema/dic/services"
           xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
           xsi:schemaLocation="http://symfony.com/schema/dic/services http://symfony.com/schema/dic/services/services-1.0.xsd">
    <services>
        <service id="Test\TestPlugin\Service\TestService">
            <argument type="service" id="logger"/>
            <tag name="kernel.event_subscriber"/>
        </service>
    </services>
</container>`

const servicesYAMLFixture = `services:
    Test\TestPlugin\Service\TestService:
        arguments:
            - '@logger'
        tags:
            - kernel.event_subscriber
`

func writeTestPlugin(t *testing.T, root string) {
	t.Helper()

	composerJSON := `{
    "name": "test/test-plugin",
    "type": "shopware-platform-plugin",
    "version": "1.0.0",
    "license": "MIT",
    "description": "Test plugin",
    "require": {
        "shopware/core": "~6.6.0"
    },
    "autoload": {
        "psr-4": {
            "Test\\TestPlugin\\": "src/"
        }
    },
    "extra": {
        "shopware-plugin-class": "Test\\TestPlugin\\TestPlugin",
        "label": {
            "en-GB": "Test Plugin"
        }
    }
}`

	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(composerJSON), 0o644))
}

func TestServicesXMLConverterIsRegistered(t *testing.T) {
	_, err := GetTools().Only("services-xml")
	assert.NoError(t, err)
}

func TestServicesXMLConverterFixesPlugin(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestPlugin(t, tmpDir)

	configDir := filepath.Join(tmpDir, "src", "Resources", "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "services.xml"), []byte(servicesXMLFixture), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(configDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, servicesYAMLFixture, string(content))
}

func TestServicesXMLConverterFixesExtraBundles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestPlugin(t, tmpDir)

	extensionConfig := `build:
  extraBundles:
    - path: Foo
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yml"), []byte(extensionConfig), 0o644))

	bundleConfigDir := filepath.Join(tmpDir, "src", "Foo", "Resources", "config")
	require.NoError(t, os.MkdirAll(bundleConfigDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bundleConfigDir, "services.xml"), []byte(servicesXMLFixture), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(bundleConfigDir, "services.xml"))
	assert.FileExists(t, filepath.Join(bundleConfigDir, "services.yaml"))
}

func TestServicesXMLConverterSkipsApps(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
    <meta>
        <name>TestApp</name>
        <label>Test App</label>
        <description>A test app</description>
        <author>Test Author</author>
        <copyright>(c) Test</copyright>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
</manifest>`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(manifest), 0o644))

	configDir := filepath.Join(tmpDir, "Resources", "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "services.xml"), []byte(servicesXMLFixture), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestServicesXMLConverterKeepsUnconvertibleFile(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestPlugin(t, tmpDir)

	configDir := filepath.Join(tmpDir, "src", "Resources", "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "services.xml"), []byte(`<container>
    <services>
        <stack id="not-supported"/>
    </services>
</container>`), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	err = ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{Extension: ext, RootDir: tmpDir})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestServicesXMLConverterFixesProjectPlugins(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "custom", "plugins", "TestPlugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	writeTestPlugin(t, pluginDir)

	configDir := filepath.Join(pluginDir, "src", "Resources", "config")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "services.xml"), []byte(servicesXMLFixture), 0o644))

	err := ServicesXMLConverter{}.Fix(t.Context(), ToolConfig{RootDir: tmpDir})
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))
	assert.FileExists(t, filepath.Join(configDir, "services.yaml"))
}
