package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindProjectRoot(t *testing.T) {
	t.Run("falls back to current directory when no config path", func(t *testing.T) {
		// Create a temporary Shopware project structure
		tmpDir := t.TempDir()

		// Create composer.json with shopware/core dependency
		composerJson := `{
			"require": {
				"shopware/core": "~6.4.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		// Create bin/console
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		// Change to the temp directory
		oldDir, err := os.Getwd()
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, os.Chdir(oldDir))
		}()
		assert.NoError(t, os.Chdir(tmpDir))

		// Test with empty config path (should use current directory)
		project, err := findProjectRoot("")
		assert.NoError(t, err)
		assert.Equal(t, tmpDir, project)
	})

	t.Run("derives project root from absolute config path", func(t *testing.T) {
		// Create a temporary Shopware project structure
		tmpDir := t.TempDir()

		// Create composer.json with shopware/core dependency
		composerJson := `{
			"require": {
				"shopware/core": "~6.4.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		// Create bin/console
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		// Create a config file in the project directory
		configPath := filepath.Join(tmpDir, ".shopware-project.yml")
		configContent := `url: http://localhost:8000`
		assert.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		// Test with absolute config path
		project, err := findProjectRoot(configPath)
		assert.NoError(t, err)
		assert.Equal(t, tmpDir, project)
	})

	t.Run("falls back when config directory is not a shopware project", func(t *testing.T) {
		// Create a temporary directory that's NOT a Shopware project
		tmpDir := t.TempDir()

		// Create a config file but no Shopware project structure
		configPath := filepath.Join(tmpDir, ".shopware-project.yml")
		configContent := `url: http://localhost:8000`
		assert.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		// Test with absolute config path - should fall back to findClosestShopwareProject
		_, err := findProjectRoot(configPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot find Shopware project")
	})

	t.Run("ignores relative config paths", func(t *testing.T) {
		// Create a temporary Shopware project structure
		tmpDir := t.TempDir()

		// Create composer.json with shopware/core dependency
		composerJson := `{
			"require": {
				"shopware/core": "~6.4.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		// Create bin/console
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		// Change to the temp directory
		oldDir, err := os.Getwd()
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, os.Chdir(oldDir))
		}()
		assert.NoError(t, os.Chdir(tmpDir))

		// Test with relative config path (should ignore it and use current directory)
		project, err := findProjectRoot(".shopware-project.yml")
		assert.NoError(t, err)
		assert.Equal(t, tmpDir, project)
	})
}

func TestIsShopwareProject(t *testing.T) {
	t.Run("detects valid shopware project with composer.json", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create composer.json with shopware/core dependency
		composerJson := `{
			"require": {
				"shopware/core": "~6.4.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		// Create bin/console
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		assert.True(t, isShopwareProject(tmpDir))
	})

	t.Run("detects valid shopware project with composer.lock", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create composer.lock with shopware/core dependency
		composerLock := `{
			"packages": [
				{
					"name": "shopware/core",
					"version": "v6.4.0.0"
				}
			]
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.lock"), []byte(composerLock), 0644))

		// Create bin/console
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		assert.True(t, isShopwareProject(tmpDir))
	})

	t.Run("rejects directory without shopware/core", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create composer.json without shopware/core dependency
		composerJson := `{
			"require": {
				"symfony/console": "^5.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		assert.False(t, isShopwareProject(tmpDir))
	})

	t.Run("rejects directory without bin/console", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create composer.json with shopware/core dependency
		composerJson := `{
			"require": {
				"shopware/core": "~6.4.0"
			}
		}`
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(composerJson), 0644))

		// Don't create bin/console

		assert.False(t, isShopwareProject(tmpDir))
	})

	t.Run("rejects directory without composer files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create bin/console but no composer files
		binDir := filepath.Join(tmpDir, "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(binDir, "console"), []byte("#!/usr/bin/env php"), 0755))

		assert.False(t, isShopwareProject(tmpDir))
	})
}
