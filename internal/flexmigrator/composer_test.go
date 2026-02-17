package flexmigrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestMigrateComposerJson(t *testing.T) {
	t.Parallel()
	t.Run("successful migration", func(t *testing.T) {
		t.Parallel()
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create a test composer.json file
		initialComposer := &packagist.ComposerJson{
			Name: "shopware/project",
			Require: packagist.ComposerPackageLink{
				"shopware/recovery": "1.0.0",
				"php":               "^7.4",
			},
			RequireDev: packagist.ComposerPackageLink{
				"some/dev-package": "^1.0",
			},
			Config: map[string]any{
				"platform": map[string]string{
					"php": "7.4",
				},
				"allow-plugins": map[string]any{
					"composer/package-versions-deprecated": true,
				},
			},
			Repositories: packagist.ComposerJsonRepositories{},
			Scripts:      map[string]any{},
			Extra:        map[string]any{},
		}

		composerFile := filepath.Join(tempDir, "composer.json")
		content, err := json.MarshalIndent(initialComposer, "", "  ")
		require.NoError(t, err)
		err = os.WriteFile(composerFile, content, 0o644)
		require.NoError(t, err)

		// Run the migration
		err = MigrateComposerJson(tempDir)
		require.NoError(t, err)

		// Read and verify the migrated composer.json
		migratedComposer, err := packagist.ReadComposerJson(composerFile)
		require.NoError(t, err)

		// Verify package removals
		assert.False(t, migratedComposer.HasPackage("shopware/recovery"))
		assert.False(t, migratedComposer.HasPackage("php"))

		// Verify package additions
		assert.Equal(t, "^2", migratedComposer.Require["symfony/flex"])
		assert.Equal(t, "*", migratedComposer.Require["symfony/runtime"])
		assert.Equal(t, "*", migratedComposer.RequireDev["shopware/dev-tools"])

		// Verify config changes
		assert.False(t, migratedComposer.HasConfig("platform"))

		// Verify plugin configuration
		allowPlugins, ok := migratedComposer.Config["allow-plugins"].(map[string]interface{})
		require.True(t, ok)
		flexEnabled, ok := allowPlugins["symfony/flex"]
		require.True(t, ok)
		assert.Equal(t, true, flexEnabled)
		runtimeEnabled, ok := allowPlugins["symfony/runtime"]
		require.True(t, ok)
		assert.Equal(t, true, runtimeEnabled)
		_, hasDeprecatedPlugin := allowPlugins["composer/package-versions-deprecated"]
		assert.False(t, hasDeprecatedPlugin)

		// Verify symfony configuration
		symfonyConfig, ok := migratedComposer.Extra["symfony"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, true, symfonyConfig["allow-contrib"])
		endpoints, ok := symfonyConfig["endpoint"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, endpoints, "https://raw.githubusercontent.com/shopware/recipes/flex/main/index.json")
		assert.Contains(t, endpoints, "flex://defaults")

		// Verify repository configuration
		assert.True(t, migratedComposer.Repositories.HasRepository("custom/plugins/*"))
		assert.True(t, migratedComposer.Repositories.HasRepository("custom/plugins/*/packages/*"))

		// Verify scripts configuration
		autoScripts, ok := migratedComposer.Scripts["auto-scripts"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "symfony-cmd", autoScripts["assets:install"])

		postInstallCmd, ok := migratedComposer.Scripts["post-install-cmd"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, postInstallCmd, "@auto-scripts")

		postUpdateCmd, ok := migratedComposer.Scripts["post-update-cmd"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, postUpdateCmd, "@auto-scripts")
	})

	t.Run("non-existent composer.json", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		err := MigrateComposerJson(tempDir)
		assert.Error(t, err)
	})

	t.Run("invalid composer.json", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")
		err := os.WriteFile(composerFile, []byte("invalid json"), 0o644)
		require.NoError(t, err)

		err = MigrateComposerJson(tempDir)
		assert.Error(t, err)
	})
}
