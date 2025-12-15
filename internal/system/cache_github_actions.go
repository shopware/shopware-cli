package system

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	actionscache "github.com/tonistiigi/go-actions-cache"
)

// GitHubActionsCache implements Cache interface using GitHub Actions cache
type GitHubActionsCache struct {
	client    *actionscache.Cache
	prefix    string
	tempFiles []string
	mu        sync.Mutex
}

// NewGitHubActionsCache creates a new GitHub Actions cache
func NewGitHubActionsCache(prefix string) (*GitHubActionsCache, error) {
	client, err := actionscache.TryEnv(actionscache.Opt{})
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub Actions cache client: %w", err)
	}

	if client == nil {
		return nil, fmt.Errorf("GitHub Actions cache client is not available")
	}

	return &GitHubActionsCache{
		client:    client,
		prefix:    prefix,
		tempFiles: make([]string, 0),
	}, nil
}

// Get retrieves a cached item by key
func (c *GitHubActionsCache) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	cacheKey := c.getCacheKey(key)

	entry, err := c.client.Load(ctx, cacheKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load from GitHub Actions cache: %w", err)
	}

	if entry == nil {
		return nil, ErrCacheNotFound
	}

	// Download the entry content to a buffer
	var buf bytes.Buffer
	if err := entry.WriteTo(ctx, &buf); err != nil {
		return nil, fmt.Errorf("failed to download cache entry: %w", err)
	}

	return io.NopCloser(&buf), nil
}

// Set stores an item in the cache with the given key
func (c *GitHubActionsCache) Set(ctx context.Context, key string, data io.Reader) error {
	cacheKey := c.getCacheKey(key)

	// Read all data into memory to create a Blob
	dataBytes, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	blob := actionscache.NewBlob(dataBytes)

	err = c.client.Save(ctx, cacheKey, blob)
	if err != nil {
		return fmt.Errorf("failed to save to GitHub Actions cache: %w", err)
	}

	return nil
}

// GetFilePath downloads the cache item to a temporary file and returns the file path
func (c *GitHubActionsCache) GetFilePath(ctx context.Context, key string) (string, error) {
	cacheKey := c.getCacheKey(key)

	entry, err := c.client.Load(ctx, cacheKey)
	if err != nil {
		return "", fmt.Errorf("failed to load from GitHub Actions cache: %w", err)
	}

	if entry == nil {
		return "", ErrCacheNotFound
	}

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "gh-actions-cache-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	// Download the entry content to the temporary file
	if err := entry.WriteTo(ctx, tmpFile); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to download cache entry to temp file: %w", err)
	}

	// Track the temporary file for cleanup
	c.mu.Lock()
	c.tempFiles = append(c.tempFiles, tmpFile.Name())
	c.mu.Unlock()

	return tmpFile.Name(), nil
}

// StoreFolderCache stores an entire folder structure in the cache
func (c *GitHubActionsCache) StoreFolderCache(ctx context.Context, key string, folderPath string) error {
	cacheKey := c.getCacheKey(key + "-folder")

	// Create tar.gz archive in memory
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	// Walk through the folder and add all files to the tar
	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from the base folder
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create tar header with proper symlink handling
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

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content if it's a regular file
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

	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	// Close writers to flush data
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Store the archive in cache
	blob := actionscache.NewBlob(buf.Bytes())
	if err := c.client.Save(ctx, cacheKey, blob); err != nil {
		return fmt.Errorf("failed to save folder to GitHub Actions cache: %w", err)
	}

	return nil
}

// GetFolderCachePath downloads and extracts the cached folder to a temporary directory
func (c *GitHubActionsCache) GetFolderCachePath(ctx context.Context, key string) (string, error) {
	cacheKey := c.getCacheKey(key + "-folder")

	entry, err := c.client.Load(ctx, cacheKey)
	if err != nil {
		return "", fmt.Errorf("failed to load from GitHub Actions cache: %w", err)
	}

	if entry == nil {
		return "", ErrCacheNotFound
	}

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "gh-actions-folder-cache-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Download the archive to memory
	var buf bytes.Buffer
	if err := entry.WriteTo(ctx, &buf); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to download cache entry: %w", err)
	}

	// Extract the tar.gz archive
	if err := c.extractTarGzFromBytes(buf.Bytes(), tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to extract cached folder: %w", err)
	}

	// Track the temporary directory for cleanup
	c.mu.Lock()
	c.tempFiles = append(c.tempFiles, tmpDir)
	c.mu.Unlock()

	return tmpDir, nil
}

// extractTarGzFromBytes extracts a tar.gz archive from bytes to the specified directory
func (c *GitHubActionsCache) extractTarGzFromBytes(data []byte, extractPath string) error {
	// Create gzip reader from bytes
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() {
		_ = gzipReader.Close()
	}()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Extract all files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Sanitize the path to prevent directory traversal
		if strings.Contains(header.Name, "..") {
			continue
		}

		targetPath := filepath.Join(extractPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}

			// Create the file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}

			// Copy file content
			if _, err := io.Copy(outFile, tarReader); err != nil {
				_ = outFile.Close()
				return err
			}

			// Set file permissions
			if err := outFile.Chmod(os.FileMode(header.Mode)); err != nil {
				_ = outFile.Close()
				return err
			}

			if err := outFile.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Create directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}

			// Create the symlink
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// RestoreFolderCache downloads and extracts a cached folder to the specified target directory
func (c *GitHubActionsCache) RestoreFolderCache(ctx context.Context, key string, targetPath string) error {
	cacheKey := c.getCacheKey(key + "-folder")

	entry, err := c.client.Load(ctx, cacheKey)
	if err != nil {
		return fmt.Errorf("failed to load from GitHub Actions cache: %w", err)
	}

	if entry == nil {
		return ErrCacheNotFound
	}

	// Download the archive to memory
	var buf bytes.Buffer
	if err := entry.WriteTo(ctx, &buf); err != nil {
		return fmt.Errorf("failed to download cache entry: %w", err)
	}

	// Extract the tar.gz archive to the target path
	if err := c.extractTarGzFromBytes(buf.Bytes(), targetPath); err != nil {
		return fmt.Errorf("failed to extract cached folder: %w", err)
	}

	return nil
}

// Close cleans up all temporary files created by GetFilePath
func (c *GitHubActionsCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errors []error
	for _, tempPath := range c.tempFiles {
		// Try to remove as file first, then as directory
		if err := os.Remove(tempPath); err != nil {
			if err := os.RemoveAll(tempPath); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Errorf("failed to remove temp path %s: %w", tempPath, err))
			}
		}
	}

	// Clear the temp files list
	c.tempFiles = c.tempFiles[:0]

	if len(errors) > 0 {
		return fmt.Errorf("failed to clean up some temp paths: %v", errors)
	}

	return nil
}

// getCacheKey creates a cache key with the prefix and ensures it's valid
func (c *GitHubActionsCache) getCacheKey(key string) string {
	// GitHub Actions cache keys have restrictions, so we hash the key
	hash := sha256.Sum256([]byte(key))
	hashStr := fmt.Sprintf("%x", hash)

	// Combine prefix with hash, ensuring valid characters
	cacheKey := fmt.Sprintf("%s-%s", c.prefix, hashStr)

	// GitHub Actions cache keys can't contain certain characters
	cacheKey = strings.ReplaceAll(cacheKey, "/", "-")
	cacheKey = strings.ReplaceAll(cacheKey, "\\", "-")

	return cacheKey
}
