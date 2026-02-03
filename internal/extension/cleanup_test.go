package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupExtensionFolder_RemovesDefaultNotAllowedPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some default not-allowed paths
	pathsToCreate := []string{
		".git",
		".github",
		"tests",
		"var",
		"phpstan.neon",
		"Makefile",
	}

	for _, p := range pathsToCreate {
		fullPath := filepath.Join(tmpDir, p)
		if filepath.Ext(p) == "" && !contains([]string{"phpstan.neon", "Makefile"}, p) {
			require.NoError(t, os.MkdirAll(fullPath, 0755))
			// Add a file inside directories
			require.NoError(t, os.WriteFile(filepath.Join(fullPath, "test.txt"), []byte("test"), 0644))
		} else {
			require.NoError(t, os.WriteFile(fullPath, []byte("test"), 0644))
		}
	}

	// Create a file that should remain
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte("{}"), 0644))

	err := CleanupExtensionFolder(tmpDir, nil)
	require.NoError(t, err)

	// Verify not-allowed paths are removed
	for _, p := range pathsToCreate {
		_, err := os.Stat(filepath.Join(tmpDir, p))
		assert.True(t, os.IsNotExist(err), "Expected %s to be removed", p)
	}

	// Verify allowed file remains
	_, err = os.Stat(filepath.Join(tmpDir, "composer.json"))
	assert.NoError(t, err, "composer.json should remain")
}

func TestCleanupExtensionFolder_RemovesNotAllowedFilesInSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory structure
	subDir := filepath.Join(tmpDir, "src", "Resources")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	// Create not-allowed files in subdirectories
	notAllowedFiles := []string{
		filepath.Join(tmpDir, ".DS_Store"),
		filepath.Join(subDir, ".DS_Store"),
		filepath.Join(subDir, ".gitignore"),
		filepath.Join(subDir, ".gitkeep"),
		filepath.Join(subDir, "eslint.config.js"),
	}

	for _, f := range notAllowedFiles {
		require.NoError(t, os.WriteFile(f, []byte("test"), 0644))
	}

	// Create a file that should remain
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "index.js"), []byte("test"), 0644))

	err := CleanupExtensionFolder(tmpDir, nil)
	require.NoError(t, err)

	// Verify not-allowed files are removed
	for _, f := range notAllowedFiles {
		_, err := os.Stat(f)
		assert.True(t, os.IsNotExist(err), "Expected %s to be removed", f)
	}

	// Verify allowed file remains
	_, err = os.Stat(filepath.Join(subDir, "index.js"))
	assert.NoError(t, err, "index.js should remain")
}

func TestCleanupExtensionFolder_RemovesNotAllowedExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with not-allowed extensions
	notAllowedExtFiles := []string{
		"archive.zip",
		"backup.tar",
		"data.tar.gz",
		"tool.phar",
	}

	for _, f := range notAllowedExtFiles {
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644))
	}

	// Create a file that should remain
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.js"), []byte("test"), 0644))

	err := CleanupExtensionFolder(tmpDir, nil)
	require.NoError(t, err)

	// Verify not-allowed extension files are removed
	for _, f := range notAllowedExtFiles {
		_, err := os.Stat(filepath.Join(tmpDir, f))
		assert.True(t, os.IsNotExist(err), "Expected %s to be removed", f)
	}

	// Verify allowed file remains
	_, err = os.Stat(filepath.Join(tmpDir, "main.js"))
	assert.NoError(t, err, "main.js should remain")
}

func TestCleanupExtensionFolder_WithAdditionalPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create custom paths that should be removed
	customPaths := []string{
		"custom-build",
		"node_modules",
	}

	for _, p := range customPaths {
		fullPath := filepath.Join(tmpDir, p)
		require.NoError(t, os.MkdirAll(fullPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(fullPath, "file.txt"), []byte("test"), 0644))
	}

	// Create a file that should remain
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "src.js"), []byte("test"), 0644))

	err := CleanupExtensionFolder(tmpDir, customPaths)
	require.NoError(t, err)

	// Verify custom paths are removed
	for _, p := range customPaths {
		_, err := os.Stat(filepath.Join(tmpDir, p))
		assert.True(t, os.IsNotExist(err), "Expected %s to be removed", p)
	}

	// Verify allowed file remains
	_, err = os.Stat(filepath.Join(tmpDir, "src.js"))
	assert.NoError(t, err)
}

func TestCleanupExtensionFolder_DoesNotMutateGlobalSlice(t *testing.T) {
	tmpDir := t.TempDir()

	originalLen := len(defaultNotAllowedPaths)

	// Call with additional paths
	err := CleanupExtensionFolder(tmpDir, []string{"extra1", "extra2"})
	require.NoError(t, err)

	// Verify global slice was not mutated
	assert.Equal(t, originalLen, len(defaultNotAllowedPaths), "defaultNotAllowedPaths should not be mutated")
}

func TestCleanupExtensionFolder_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	err := CleanupExtensionFolder(tmpDir, nil)
	require.NoError(t, err)
}

func TestCleanupExtensionFolder_NestedNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested node_modules that matches default path
	nestedPath := filepath.Join(tmpDir, "src", "Resources", "app", "administration", "node_modules")
	require.NoError(t, os.MkdirAll(nestedPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nestedPath, "package.json"), []byte("{}"), 0644))

	err := CleanupExtensionFolder(tmpDir, nil)
	require.NoError(t, err)

	// The exact path src/Resources/app/administration/node_modules should be removed
	_, err = os.Stat(nestedPath)
	assert.True(t, os.IsNotExist(err), "node_modules should be removed")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
