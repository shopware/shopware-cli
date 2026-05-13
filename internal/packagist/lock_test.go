package packagist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadComposerLock(t *testing.T) {
	t.Run("valid composer.lock", func(t *testing.T) {
		// Create a temporary composer.lock file
		dir := t.TempDir()
		lockFile := filepath.Join(dir, "composer.lock")
		content := `{
			"packages": [
				{
					"name": "symfony/console",
					"version": "v6.3.0",
					"type": "library"
				}
			]
		}`
		err := os.WriteFile(lockFile, []byte(content), 0o644)
		assert.NoError(t, err)

		// Test reading the file
		lock, err := ReadComposerLock(lockFile)
		assert.NoError(t, err)
		assert.NotNil(t, lock)
		assert.Len(t, lock.Packages, 1)
		assert.Equal(t, "symfony/console", lock.Packages[0].Name)
		assert.Equal(t, "v6.3.0", lock.Packages[0].Version)
	})

	t.Run("non-existent file", func(t *testing.T) {
		lock, err := ReadComposerLock("non-existent-file.lock")
		assert.Error(t, err)
		assert.Nil(t, lock)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// Create a temporary file with invalid JSON
		dir := t.TempDir()
		lockFile := filepath.Join(dir, "invalid.lock")
		err := os.WriteFile(lockFile, []byte("invalid json"), 0o644)
		assert.NoError(t, err)

		lock, err := ReadComposerLock(lockFile)
		assert.Error(t, err)
		assert.Nil(t, lock)
	})
}

func TestShopwarePHPConstraint(t *testing.T) {
	t.Run("from shopware/core", func(t *testing.T) {
		lock := &ComposerLock{
			Packages: []ComposerLockPackage{
				{Name: "symfony/console", Version: "v6.3.0"},
				{Name: "shopware/core", Version: "v6.6.10.0", Require: map[string]string{"php": "~8.2.0 || ~8.3.0"}},
			},
		}
		c := lock.ShopwarePHPConstraint()
		assert.NotNil(t, c)
		assert.Equal(t, "~8.2.0 || ~8.3.0", c.String())
	})

	t.Run("falls back to shopware/platform", func(t *testing.T) {
		lock := &ComposerLock{
			Packages: []ComposerLockPackage{
				{Name: "shopware/platform", Version: "v6.5.0.0", Require: map[string]string{"php": ">=8.1"}},
			},
		}
		c := lock.ShopwarePHPConstraint()
		assert.NotNil(t, c)
		assert.Equal(t, ">=8.1", c.String())
	})

	t.Run("prefers core over platform", func(t *testing.T) {
		lock := &ComposerLock{
			Packages: []ComposerLockPackage{
				{Name: "shopware/platform", Version: "v6.5.0.0", Require: map[string]string{"php": ">=8.1"}},
				{Name: "shopware/core", Version: "v6.6.10.0", Require: map[string]string{"php": ">=8.2"}},
			},
		}
		assert.Equal(t, ">=8.2", lock.ShopwarePHPConstraint().String())
	})

	t.Run("returns nil when no shopware package present", func(t *testing.T) {
		lock := &ComposerLock{Packages: []ComposerLockPackage{{Name: "symfony/console"}}}
		assert.Nil(t, lock.ShopwarePHPConstraint())
	})

	t.Run("returns nil when shopware package has no php require", func(t *testing.T) {
		lock := &ComposerLock{
			Packages: []ComposerLockPackage{{Name: "shopware/core", Version: "v6.6.0.0"}},
		}
		assert.Nil(t, lock.ShopwarePHPConstraint())
	})
}
