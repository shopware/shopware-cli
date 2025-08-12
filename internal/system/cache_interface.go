package system

import (
	"context"
	"errors"
	"io"
)

// ErrCacheNotFound is returned when a cache item cannot be found
var ErrCacheNotFound = errors.New("cache item not found")

// Cache provides a unified interface for caching operations
type Cache interface {
	// Get retrieves a cached item by key. Returns ErrCacheNotFound if not found.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Set stores an item in the cache with the given key
	Set(ctx context.Context, key string, data io.Reader) error

	// GetFilePath returns a file path for direct access to the cached item.
	// For local caches, this returns the actual file path.
	// For remote caches, this downloads the item to a temp file and returns that path.
	// Returns ErrCacheNotFound if the item doesn't exist.
	GetFilePath(ctx context.Context, key string) (string, error)

	// StoreFolderCache stores an entire folder structure in the cache with the given key.
	// The folderPath should be the root directory to cache.
	StoreFolderCache(ctx context.Context, key string, folderPath string) error

	// GetFolderCachePath returns a folder path for direct access to the cached folder.
	// For local caches, this returns the actual folder path.
	// For remote caches, this downloads and extracts the folder to a temp directory.
	// Returns ErrCacheNotFound if the folder doesn't exist.
	GetFolderCachePath(ctx context.Context, key string) (string, error)

	// RestoreFolderCache extracts/copies a cached folder to the specified target directory.
	// This is useful when you want to restore cached content to a specific location.
	// Returns ErrCacheNotFound if the cached folder doesn't exist.
	RestoreFolderCache(ctx context.Context, key string, targetPath string) error

	// Close cleans up any temporary files or resources held by the cache
	Close() error
}

// CacheFactory creates cache instances based on the environment
type CacheFactory interface {
	CreateCache() Cache
}
