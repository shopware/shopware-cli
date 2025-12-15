package system

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DiskCache implements Cache interface using local filesystem
type DiskCache struct {
	basePath string
}

// NewDiskCache creates a new disk-based cache
func NewDiskCache(basePath string) *DiskCache {
	return &DiskCache{
		basePath: basePath,
	}
}

// Get retrieves a cached item by key
func (c *DiskCache) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	filePath := c.getFilePath(key)

	file, err := os.Open(filePath)
	if os.IsNotExist(err) {
		return nil, ErrCacheNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open cached file: %w", err)
	}

	return file, nil
}

// Set stores an item in the cache with the given key
func (c *DiskCache) Set(ctx context.Context, key string, data io.Reader) error {
	filePath := c.getFilePath(key)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create temporary file first
	tmpPath := filePath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	// Copy data to temporary file
	if _, err := io.Copy(tmpFile, data); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write data to cache: %w", err)
	}

	// Atomically rename temporary file to final location
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize cached file: %w", err)
	}

	return nil
}

// GetFilePath returns the file path for direct access to the cached item
func (c *DiskCache) GetFilePath(ctx context.Context, key string) (string, error) {
	filePath := c.getFilePath(key)

	// Check if file exists
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return "", ErrCacheNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to check cached file: %w", err)
	}

	return filePath, nil
}

// StoreFolderCache stores an entire folder structure in the cache
func (c *DiskCache) StoreFolderCache(ctx context.Context, key string, folderPath string) error {
	// Get the cache folder path
	cacheFolderPath := c.getFolderPath(key)

	// Create a temporary folder first for atomic operation
	tmpPath := cacheFolderPath + ".tmp"

	// Remove any existing temporary folder
	_ = os.RemoveAll(tmpPath)

	// Copy the folder to temporary location
	if err := CopyFiles(folderPath, tmpPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		return fmt.Errorf("failed to copy folder to cache: %w", err)
	}

	// Remove any existing cached folder
	_ = os.RemoveAll(cacheFolderPath)

	// Ensure parent directory exists for the final location
	if err := os.MkdirAll(filepath.Dir(cacheFolderPath), 0755); err != nil {
		_ = os.RemoveAll(tmpPath)
		return fmt.Errorf("failed to create cache directory structure: %w", err)
	}

	// Atomically rename temporary folder to final location
	if err := os.Rename(tmpPath, cacheFolderPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		return fmt.Errorf("failed to finalize cached folder: %w", err)
	}

	return nil
}

// GetFolderCachePath returns the folder path for direct access to the cached folder
func (c *DiskCache) GetFolderCachePath(ctx context.Context, key string) (string, error) {
	folderPath := c.getFolderPath(key)

	// Check if folder exists
	_, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		return "", ErrCacheNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to check cached folder: %w", err)
	}

	return folderPath, nil
}

// RestoreFolderCache copies a cached folder to the specified target directory
func (c *DiskCache) RestoreFolderCache(ctx context.Context, key string, targetPath string) error {
	folderPath := c.getFolderPath(key)

	// Check if cached folder exists
	_, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		return ErrCacheNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to check cached folder: %w", err)
	}

	// Copy cached folder to target location
	if err := CopyFiles(folderPath, targetPath); err != nil {
		return fmt.Errorf("failed to restore cached folder: %w", err)
	}

	return nil
}

// Close cleans up any resources (no-op for disk cache)
func (c *DiskCache) Close() error {
	// Disk cache doesn't need cleanup
	return nil
}

// getFilePath converts a cache key to a file path
func (c *DiskCache) getFilePath(key string) string {
	// Hash the key to create a safe filename
	hash := sha256.Sum256([]byte(key))
	filename := fmt.Sprintf("%x", hash)

	// Use the first two characters as subdirectory for better distribution
	subdir := filename[:2]

	return filepath.Join(c.basePath, subdir, filename)
}

// getFolderPath converts a cache key to a folder path
func (c *DiskCache) getFolderPath(key string) string {
	// Hash the key to create a safe folder name
	hash := sha256.Sum256([]byte(key))
	foldername := fmt.Sprintf("%x", hash)

	// Use the first two characters as subdirectory for better distribution
	subdir := foldername[:2]

	return filepath.Join(c.basePath, "folders", subdir, foldername)
}
