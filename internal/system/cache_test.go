package system

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskCache(t *testing.T) {
	t.Parallel()
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	cache := NewDiskCache(tmpDir)
	ctx := context.Background()

	// Test Set and Get
	testKey := "test-key"
	testData := "test data content"

	err := cache.Set(ctx, testKey, strings.NewReader(testData))
	require.NoError(t, err)

	// Test Get existing key
	reader, err := cache.Get(ctx, testKey)
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))

	// Test Get non-existent key returns ErrCacheNotFound
	reader, err = cache.Get(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrCacheNotFound)
	assert.Nil(t, reader)

	// Test GetFilePath for existing key
	filePath, err := cache.GetFilePath(ctx, testKey)
	require.NoError(t, err)
	assert.NotEmpty(t, filePath)

	// Verify the file exists and has correct content
	fileData, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, testData, string(fileData))

	// Test GetFilePath for non-existent key
	filePath, err = cache.GetFilePath(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrCacheNotFound)
	assert.Empty(t, filePath)

	// Test Close (should be no-op for disk cache)
	err = cache.Close()
	require.NoError(t, err)
}

func TestDiskCacheFilePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cache := NewDiskCache(tmpDir)

	// Test that file path is generated correctly
	testKey := "test/key/with/slashes"
	filePath := cache.getFilePath(testKey)

	// Should create subdirectories
	assert.Contains(t, filePath, tmpDir)
	assert.True(t, len(filepath.Base(filePath)) > 0)

	// Test setting and getting with complex key
	ctx := context.Background()
	testData := "test data"

	err := cache.Set(ctx, testKey, strings.NewReader(testData))
	require.NoError(t, err)

	reader, err := cache.Get(ctx, testKey)
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testData, string(data))
}

func TestCacheFactory(t *testing.T) {
	t.Parallel()
	factory := NewCacheFactory()

	// Test default cache creation
	cache := factory.CreateCache()
	assert.NotNil(t, cache)

	// Test cache with prefix
	prefixCache := factory.CreateCacheWithPrefix("test-prefix")
	assert.NotNil(t, prefixCache)

	// Test convenience functions
	defaultCache := GetDefaultCache()
	assert.NotNil(t, defaultCache)

	prefixCache2 := GetCacheWithPrefix("another-prefix")
	assert.NotNil(t, prefixCache2)
}

func TestIsGitHubActions(t *testing.T) {
	// Test with GITHUB_ACTIONS=true
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_WORKFLOW", "")
	assert.True(t, isGitHubActions())

	// Test with CI=true and GITHUB_WORKFLOW set
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_WORKFLOW", "test-workflow")
	assert.True(t, isGitHubActions())

	// Test with neither condition met
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_WORKFLOW", "")
	assert.False(t, isGitHubActions())

	// Test with CI=true but no GITHUB_WORKFLOW
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_WORKFLOW", "")
	assert.False(t, isGitHubActions())
}

func TestCacheInterfaceCompliance(t *testing.T) {
	t.Parallel()
	// Test that both implementations satisfy the Cache interface
	tmpDir := t.TempDir()

	// Test disk cache
	var diskCache Cache = NewDiskCache(tmpDir)
	ctx := context.Background()

	// Test basic operations
	testKey := "interface-test"
	testData := "interface test data"

	err := diskCache.Set(ctx, testKey, strings.NewReader(testData))
	require.NoError(t, err)

	reader, err := diskCache.Get(ctx, testKey)
	require.NoError(t, err)
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()

	filePath, err := diskCache.GetFilePath(ctx, testKey)
	require.NoError(t, err)
	assert.NotEmpty(t, filePath)

	err = diskCache.Close()
	require.NoError(t, err)
}

func TestDiskCacheFolderOperations(t *testing.T) {
	t.Parallel()
	// Create temporary directories for testing
	tmpDir := t.TempDir()
	sourceDir := t.TempDir()

	// Create some test files in the source directory
	err := os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("content1"), 0644)
	require.NoError(t, err)

	subDir := filepath.Join(sourceDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644)
	require.NoError(t, err)

	cache := NewDiskCache(tmpDir)
	ctx := context.Background()
	cacheKey := "test-folder"

	// Test StoreFolderCache
	err = cache.StoreFolderCache(ctx, cacheKey, sourceDir)
	require.NoError(t, err)

	// Test GetFolderCachePath
	extractedPath, err := cache.GetFolderCachePath(ctx, cacheKey)
	require.NoError(t, err)
	require.NotEmpty(t, extractedPath)

	// Verify extracted files exist and have correct content
	file1Path := filepath.Join(extractedPath, "file1.txt")
	file1Content, err := os.ReadFile(file1Path)
	require.NoError(t, err)
	assert.Equal(t, "content1", string(file1Content))

	file2Path := filepath.Join(extractedPath, "subdir", "file2.txt")
	file2Content, err := os.ReadFile(file2Path)
	require.NoError(t, err)
	assert.Equal(t, "content2", string(file2Content))

	// Test GetFolderCachePath for non-existent key
	_, err = cache.GetFolderCachePath(ctx, "non-existent")
	assert.ErrorIs(t, err, ErrCacheNotFound)

	// Test RestoreFolderCache
	restoreDir := t.TempDir()
	err = cache.RestoreFolderCache(ctx, cacheKey, restoreDir)
	require.NoError(t, err)

	// Verify restored files exist and have correct content
	restoredFile1 := filepath.Join(restoreDir, "file1.txt")
	restoredContent1, err := os.ReadFile(restoredFile1)
	require.NoError(t, err)
	assert.Equal(t, "content1", string(restoredContent1))

	restoredFile2 := filepath.Join(restoreDir, "subdir", "file2.txt")
	restoredContent2, err := os.ReadFile(restoredFile2)
	require.NoError(t, err)
	assert.Equal(t, "content2", string(restoredContent2))

	// Test RestoreFolderCache for non-existent key
	err = cache.RestoreFolderCache(ctx, "non-existent", t.TempDir())
	assert.ErrorIs(t, err, ErrCacheNotFound)
}

func TestDiskCacheStoreFolderCreatesParentDirectory(t *testing.T) {
	t.Parallel()
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	// Create a source directory with some content
	sourceDir := t.TempDir()
	err := os.WriteFile(filepath.Join(sourceDir, "test.txt"), []byte("test content"), 0644)
	require.NoError(t, err)

	// Create a cache instance with a nested path to ensure parent directories need to be created
	cache := NewDiskCache(tmpDir)
	ctx := context.Background()
	cacheKey := "test-folder-with-long-key-that-creates-nested-structure"

	// Store the folder cache - this should create the necessary parent directory structure
	err = cache.StoreFolderCache(ctx, cacheKey, sourceDir)
	require.NoError(t, err)

	// Verify the cached folder exists and has correct content
	cachedFolderPath, err := cache.GetFolderCachePath(ctx, cacheKey)
	require.NoError(t, err)
	require.NotEmpty(t, cachedFolderPath)

	// Check that the file exists in the cached folder
	cachedFile := filepath.Join(cachedFolderPath, "test.txt")
	cachedContent, err := os.ReadFile(cachedFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(cachedContent))

	// Verify the parent directory structure was created
	parentDir := filepath.Dir(cachedFolderPath)
	_, err = os.Stat(parentDir)
	require.NoError(t, err, "Parent directory should exist")
}

func TestGitHubActionsCacheSymlinksAndPermissions(t *testing.T) {
	t.Parallel()
	// This test would only work in GitHub Actions, but we can test the tar creation logic
	sourceDir := t.TempDir()

	// Create an executable file
	execFile := filepath.Join(sourceDir, "executable")
	err := os.WriteFile(execFile, []byte("#!/bin/bash\necho hello"), 0755)
	require.NoError(t, err)

	// Create a regular file
	regularFile := filepath.Join(sourceDir, "regular.txt")
	err = os.WriteFile(regularFile, []byte("regular content"), 0644)
	require.NoError(t, err)

	// Create a symlink (skip on Windows where symlinks might not be available)
	symlinkPath := filepath.Join(sourceDir, "symlink")
	if err := os.Symlink("regular.txt", symlinkPath); err != nil {
		if os.IsPermission(err) {
			t.Skip("Skipping symlink test due to permission issues (likely Windows)")
		}
		require.NoError(t, err)
	}

	// Create GitHub Actions cache instance
	cache := &GitHubActionsCache{
		client:    nil, // We won't actually use the client in this test
		prefix:    "test",
		tempFiles: make([]string, 0),
	}

	// Test that we can create a tar.gz archive with symlinks and proper permissions
	// We'll verify this by extracting to a temp directory
	tempDir := t.TempDir()

	// This tests the internal tar creation and extraction logic
	err = cache.extractTarGzFromBytes(createTestTarGz(t, sourceDir), tempDir)
	require.NoError(t, err)

	// Verify executable permissions
	extractedExec := filepath.Join(tempDir, "executable")
	execInfo, err := os.Stat(extractedExec)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), execInfo.Mode().Perm())

	// Verify regular file permissions
	extractedRegular := filepath.Join(tempDir, "regular.txt")
	regularInfo, err := os.Stat(extractedRegular)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), regularInfo.Mode().Perm())

	// Verify symlink (if it was created)
	extractedSymlink := filepath.Join(tempDir, "symlink")
	symlinkInfo, err := os.Lstat(extractedSymlink)
	if err == nil {
		assert.True(t, symlinkInfo.Mode()&os.ModeSymlink != 0)

		// Verify symlink target
		target, err := os.Readlink(extractedSymlink)
		require.NoError(t, err)
		assert.Equal(t, "regular.txt", target)
	}
}

// Helper function to create a tar.gz archive for testing
func createTestTarGz(t *testing.T, sourceDir string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		var linkTarget string
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() {
				_ = file.Close()
			}()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}
