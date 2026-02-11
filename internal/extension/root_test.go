package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExtensionByFolder_DetectsApp(t *testing.T) {
	tmpDir := t.TempDir()

	// Create manifest.xml for an App
	manifestContent := `<?xml version="1.0" encoding="UTF-8"?>
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
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(manifestContent), 0644))

	ext, err := GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	assert.Equal(t, TypePlatformApp, ext.GetType())

	name, err := ext.GetName()
	require.NoError(t, err)
	assert.Equal(t, "TestApp", name)
}

func TestGetExtensionByFolder_DetectsPlatformPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create composer.json for a PlatformPlugin
	composerContent := `{
    "name": "test/test-plugin",
    "type": "shopware-platform-plugin",
    "version": "1.0.0",
    "license": "MIT",
    "description": "Test plugin",
    "authors": [{"name": "Test"}],
    "require": {
        "shopware/core": "~6.5.0"
    },
    "autoload": {
        "psr-4": {
            "Test\\TestPlugin\\": "src/"
        }
    },
    "extra": {
        "shopware-plugin-class": "Test\\TestPlugin\\TestPlugin",
        "label": {
            "de-DE": "Test Plugin",
            "en-GB": "Test Plugin"
        },
        "description": {
            "de-DE": "Ein Test Plugin",
            "en-GB": "A test plugin"
        },
        "manufacturerLink": {
            "de-DE": "https://example.com",
            "en-GB": "https://example.com"
        },
        "supportLink": {
            "de-DE": "https://example.com/support",
            "en-GB": "https://example.com/support"
        }
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerContent), 0644))

	ext, err := GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	assert.Equal(t, TypePlatformPlugin, ext.GetType())

	name, err := ext.GetName()
	require.NoError(t, err)
	assert.Equal(t, "TestPlugin", name)
}

func TestGetExtensionByFolder_DetectsShopwareBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create composer.json for a ShopwareBundle
	composerContent := `{
    "name": "test/test-bundle",
    "type": "shopware-bundle",
    "version": "1.0.0",
    "license": "MIT",
    "require": {
        "shopware/core": "~6.5.0"
    },
    "autoload": {
        "psr-4": {
            "Test\\TestBundle\\": "src/"
        }
    },
    "extra": {
        "shopware-bundle-name": "TestBundle"
    }
}`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerContent), 0644))

	ext, err := GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	assert.Equal(t, TypeShopwareBundle, ext.GetType())

	name, err := ext.GetName()
	require.NoError(t, err)
	assert.Equal(t, "TestBundle", name)
}

func TestGetExtensionByFolder_RejectsShopware5Plugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create plugin.xml for a Shopware 5 plugin
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "plugin.xml"), []byte("<plugin></plugin>"), 0644))

	_, err := GetExtensionByFolder(t.Context(), tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shopware 5 is not supported")
}

func TestGetExtensionByFolder_RejectsUnknownType(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty directory - no manifest.xml, no composer.json
	_, err := GetExtensionByFolder(t.Context(), tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown extension type")
}

func TestGetExtensionByFolder_PrefersManifestOverComposer(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both manifest.xml and composer.json
	manifestContent := `<?xml version="1.0" encoding="UTF-8"?>
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
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(manifestContent), 0644))

	composerContent := `{
    "name": "test/test-plugin",
    "type": "shopware-platform-plugin",
    "version": "1.0.0"
}`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerContent), 0644))

	ext, err := GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	// Should detect as App since manifest.xml is checked first
	assert.Equal(t, TypePlatformApp, ext.GetType())
}

func TestGetShopwareVersionConstraintFromComposer(t *testing.T) {
	t.Run("uses config constraint when set", func(t *testing.T) {
		config := &Config{
			Build: ConfigBuild{
				ShopwareVersionConstraint: "~6.5.0",
			},
		}

		constraint, err := getShopwareVersionConstraintFromComposer(config, map[string]string{
			"shopware/core": "~6.4.0",
		})
		require.NoError(t, err)
		assert.NotNil(t, constraint)
	})

	t.Run("uses composer require when config not set", func(t *testing.T) {
		config := &Config{}

		constraint, err := getShopwareVersionConstraintFromComposer(config, map[string]string{
			"shopware/core": "~6.5.0",
		})
		require.NoError(t, err)
		assert.NotNil(t, constraint)
	})

	t.Run("returns error when shopware/core not in require", func(t *testing.T) {
		config := &Config{}

		_, err := getShopwareVersionConstraintFromComposer(config, map[string]string{
			"php": ">=8.1",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shopware/core is required")
	})

	t.Run("handles nil config", func(t *testing.T) {
		constraint, err := getShopwareVersionConstraintFromComposer(nil, map[string]string{
			"shopware/core": "~6.5.0",
		})
		require.NoError(t, err)
		assert.NotNil(t, constraint)
	})

	t.Run("returns error for invalid constraint in config", func(t *testing.T) {
		config := &Config{
			Build: ConfigBuild{
				ShopwareVersionConstraint: "invalid[constraint",
			},
		}

		_, err := getShopwareVersionConstraintFromComposer(config, map[string]string{})
		assert.Error(t, err)
	})

	t.Run("returns error for invalid constraint in composer", func(t *testing.T) {
		config := &Config{}

		_, err := getShopwareVersionConstraintFromComposer(config, map[string]string{
			"shopware/core": "invalid[constraint",
		})
		assert.Error(t, err)
	})
}
