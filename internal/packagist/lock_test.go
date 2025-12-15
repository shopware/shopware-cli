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
