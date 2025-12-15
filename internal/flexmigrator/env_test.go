package flexmigrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateEnv(t *testing.T) {
	t.Run("successful migration with only .env", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create a test .env file
		envContent := []byte("APP_ENV=dev\nAPP_SECRET=test")
		err := os.WriteFile(filepath.Join(tempDir, ".env"), envContent, 0o644)
		require.NoError(t, err)

		// Run the migration
		err = MigrateEnv(tempDir)
		require.NoError(t, err)

		// Verify .env.local exists with original content
		envLocalContent, err := os.ReadFile(filepath.Join(tempDir, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, envContent, envLocalContent)

		// Verify .env exists and is empty
		envNewContent, err := os.ReadFile(filepath.Join(tempDir, ".env"))
		require.NoError(t, err)
		assert.Empty(t, string(envNewContent))
	})

	t.Run("no migration needed when .env.local exists", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create both .env and .env.local files
		envContent := []byte("APP_ENV=dev\nAPP_SECRET=test")
		envLocalContent := []byte("APP_ENV=local\nAPP_SECRET=local")

		err := os.WriteFile(filepath.Join(tempDir, ".env"), envContent, 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, ".env.local"), envLocalContent, 0o644)
		require.NoError(t, err)

		// Run the migration
		err = MigrateEnv(tempDir)
		require.NoError(t, err)

		// Verify both files still exist with original content
		envLocalContentAfter, err := os.ReadFile(filepath.Join(tempDir, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, envLocalContent, envLocalContentAfter)

		envContentAfter, err := os.ReadFile(filepath.Join(tempDir, ".env"))
		require.NoError(t, err)
		assert.Equal(t, envContent, envContentAfter)
	})

	t.Run("no migration needed when no files exist", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Run the migration
		err := MigrateEnv(tempDir)
		require.NoError(t, err)

		// Verify neither file was created
		_, err = os.Stat(filepath.Join(tempDir, ".env"))
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(filepath.Join(tempDir, ".env.local"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("no migration needed when only .env.local exists", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create only .env.local file
		envLocalContent := []byte("APP_ENV=local\nAPP_SECRET=local")
		err := os.WriteFile(filepath.Join(tempDir, ".env.local"), envLocalContent, 0o644)
		require.NoError(t, err)

		// Run the migration
		err = MigrateEnv(tempDir)
		require.NoError(t, err)

		// Verify .env.local still exists with original content
		envLocalContentAfter, err := os.ReadFile(filepath.Join(tempDir, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, envLocalContent, envLocalContentAfter)

		// Verify .env was not created
		_, err = os.Stat(filepath.Join(tempDir, ".env"))
		assert.True(t, os.IsNotExist(err))
	})
}
