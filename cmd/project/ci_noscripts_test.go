package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/shop"
)

func TestCICommandNoScriptsFlag(t *testing.T) {
	t.Run("no_scripts enabled should add --no-scripts flag", func(t *testing.T) {
		tmpDir := t.TempDir()

		configContent := []byte(`
url: https://example.com
build:
  no_scripts: true
`)

		configPath := filepath.Join(tmpDir, ".shopware-project.yml")
		assert.NoError(t, os.WriteFile(configPath, configContent, 0644))

		shopCfg, err := shop.ReadConfig(configPath, true)
		assert.NoError(t, err)

		assert.NotNil(t, shopCfg.Build)
		assert.True(t, shopCfg.Build.NoScripts)

		// Simulate the flag logic from ci.go
		composerFlags := []string{"install", "--no-interaction", "--no-progress"}
		
		if shopCfg.Build.NoScripts {
			composerFlags = append(composerFlags, "--no-scripts")
		}

		assert.Contains(t, composerFlags, "--no-scripts")
	})

	t.Run("no_scripts disabled should not add --no-scripts flag", func(t *testing.T) {
		tmpDir := t.TempDir()

		configContent := []byte(`
url: https://example.com
build:
  no_scripts: false
`)

		configPath := filepath.Join(tmpDir, ".shopware-project.yml")
		assert.NoError(t, os.WriteFile(configPath, configContent, 0644))

		shopCfg, err := shop.ReadConfig(configPath, true)
		assert.NoError(t, err)

		assert.NotNil(t, shopCfg.Build)
		assert.False(t, shopCfg.Build.NoScripts)

		// Simulate the flag logic from ci.go
		composerFlags := []string{"install", "--no-interaction", "--no-progress"}
		
		if shopCfg.Build.NoScripts {
			composerFlags = append(composerFlags, "--no-scripts")
		}

		assert.NotContains(t, composerFlags, "--no-scripts")
	})

	t.Run("no_scripts not set should default to false", func(t *testing.T) {
		tmpDir := t.TempDir()

		configContent := []byte(`
url: https://example.com
build:
  disable_asset_copy: false
`)

		configPath := filepath.Join(tmpDir, ".shopware-project.yml")
		assert.NoError(t, os.WriteFile(configPath, configContent, 0644))

		shopCfg, err := shop.ReadConfig(configPath, true)
		assert.NoError(t, err)

		assert.NotNil(t, shopCfg.Build)
		assert.False(t, shopCfg.Build.NoScripts)

		// Simulate the flag logic from ci.go
		composerFlags := []string{"install", "--no-interaction", "--no-progress"}
		
		if shopCfg.Build.NoScripts {
			composerFlags = append(composerFlags, "--no-scripts")
		}

		assert.NotContains(t, composerFlags, "--no-scripts")
	})
}
