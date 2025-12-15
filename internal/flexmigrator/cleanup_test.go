package flexmigrator

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup(t *testing.T) {
	t.Run("remove existing files", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create some test files that should be removed
		filesToCreate := []string{
			".dockerignore",
			"Dockerfile",
			"README.md",
			"public/index.php",
		}

		for _, file := range filesToCreate {
			fullPath := filepath.Join(tempDir, file)
			err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
			require.NoError(t, err)
			err = os.WriteFile(fullPath, []byte("test content"), 0o644)
			require.NoError(t, err)
		}

		// Create a file that should not be removed
		err := os.WriteFile(filepath.Join(tempDir, "keep-me.txt"), []byte("keep this file"), 0o644)
		require.NoError(t, err)

		// Run cleanup
		err = Cleanup(tempDir)
		require.NoError(t, err)

		// Verify files were removed
		for _, file := range filesToCreate {
			_, err := os.Stat(filepath.Join(tempDir, file))
			assert.True(t, os.IsNotExist(err), "File %s should have been removed", file)
		}

		// Verify non-targeted file still exists
		_, err = os.Stat(filepath.Join(tempDir, "keep-me.txt"))
		assert.NoError(t, err, "File keep-me.txt should still exist")
	})

	t.Run("remove existing directories", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create some test directories that should be removed
		dirsToCreate := []string{
			".github",
			"bin",
			"config/etc",
			"public/recovery",
		}

		for _, dir := range dirsToCreate {
			fullPath := filepath.Join(tempDir, dir)
			err := os.MkdirAll(fullPath, 0o755)
			require.NoError(t, err)
			// Add a file in each directory
			err = os.WriteFile(filepath.Join(fullPath, "test.txt"), []byte("test content"), 0o644)
			require.NoError(t, err)
		}

		// Create a directory that should not be removed
		keepDir := filepath.Join(tempDir, "keep-me")
		err := os.MkdirAll(keepDir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(keepDir, "test.txt"), []byte("keep this file"), 0o644)
		require.NoError(t, err)

		// Run cleanup
		err = Cleanup(tempDir)
		require.NoError(t, err)

		// Verify directories were removed
		for _, dir := range dirsToCreate {
			_, err := os.Stat(filepath.Join(tempDir, dir))
			assert.True(t, os.IsNotExist(err), "Directory %s should have been removed", dir)
		}

		// Verify non-targeted directory still exists
		_, err = os.Stat(keepDir)
		assert.NoError(t, err, "Directory keep-me should still exist")
	})

	t.Run("remove files by MD5", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create test files with specific MD5 hashes
		testCases := []struct {
			path    string
			content []byte
			remove  bool
		}{
			{
				path:    "src/Command/SystemGenerateAppSecretCommand.php",
				content: []byte(""),
				remove:  true, // MD5: d41d8cd98f00b204e9800998ecf8427e
			},
			{
				path:    "src/Command/SystemGenerateJwtSecretCommand.php",
				content: []byte(""),
				remove:  true, // MD5: d41d8cd98f00b204e9800998ecf8427e
			},
			{
				path:    "config/packages/custom.yaml",
				content: []byte("# Custom content that should be kept"),
				remove:  false,
			},
		}

		for _, tc := range testCases {
			fullPath := filepath.Join(tempDir, tc.path)
			err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
			require.NoError(t, err)
			err = os.WriteFile(fullPath, tc.content, 0o644)
			require.NoError(t, err)

			if tc.remove {
				// Verify the MD5 matches what we expect
				hash := md5.Sum(tc.content)
				md5String := hex.EncodeToString(hash[:])
				assert.Contains(t, cleanupByMd5[tc.path], md5String, "Test case MD5 should be in cleanupByMd5")
			}
		}

		// Run cleanup
		err := Cleanup(tempDir)
		require.NoError(t, err)

		// Verify results
		for _, tc := range testCases {
			_, err := os.Stat(filepath.Join(tempDir, tc.path))
			if tc.remove {
				assert.True(t, os.IsNotExist(err), "File %s should have been removed", tc.path)
			} else {
				assert.NoError(t, err, "File %s should still exist", tc.path)
			}
		}
	})

	t.Run("handle non-existent files and directories", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Run cleanup on empty directory
		err := Cleanup(tempDir)
		assert.NoError(t, err, "Cleanup should succeed on empty directory")
	})

	t.Run("handle files with non-matching MD5", func(t *testing.T) {
		// Create a temporary directory for the test
		tempDir := t.TempDir()

		// Create a file that's in cleanupByMd5 but with different content
		filePath := "config/packages/shopware.yaml"
		fullPath := filepath.Join(tempDir, filePath)
		err := os.MkdirAll(filepath.Dir(fullPath), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte("custom content that doesn't match MD5"), 0o644)
		require.NoError(t, err)

		// Run cleanup
		err = Cleanup(tempDir)
		require.NoError(t, err)

		// Verify file still exists
		_, err = os.Stat(fullPath)
		assert.NoError(t, err, "File with non-matching MD5 should not be removed")
	})
}
