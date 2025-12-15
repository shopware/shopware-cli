package system

import (
	"os"
	"path/filepath"
)

// DefaultCacheFactory implements CacheFactory
type DefaultCacheFactory struct{}

// NewCacheFactory creates a new cache factory
func NewCacheFactory() *DefaultCacheFactory {
	return &DefaultCacheFactory{}
}

// CreateCache creates a cache instance based on the environment
func (f *DefaultCacheFactory) CreateCache() Cache {
	// Check if we're running in GitHub Actions
	if isGitHubActions() {
		// Try to create GitHub Actions cache
		if cache, err := NewGitHubActionsCache("shopware-cli"); err == nil {
			return trackCache(cache)
		}
		// Fall back to disk cache if GitHub Actions cache fails
	}

	// Default to disk cache
	cacheDir := GetShopwareCliCacheDir()
	return trackCache(NewDiskCache(cacheDir))
}

// CreateCacheWithPrefix creates a cache instance with a custom prefix/directory
func (f *DefaultCacheFactory) CreateCacheWithPrefix(prefix string) Cache {
	// Check if we're running in GitHub Actions
	if isGitHubActions() {
		// Try to create GitHub Actions cache with custom prefix
		if cache, err := NewGitHubActionsCache(prefix); err == nil {
			return trackCache(cache)
		}
		// Fall back to disk cache if GitHub Actions cache fails
	}

	// Default to disk cache with custom subdirectory
	cacheDir := filepath.Join(GetShopwareCliCacheDir(), prefix)
	return trackCache(NewDiskCache(cacheDir))
}

// isGitHubActions detects if we're running in GitHub Actions environment
func isGitHubActions() bool {
	// GitHub Actions sets these environment variables
	return os.Getenv("GITHUB_ACTIONS") == "true" ||
		os.Getenv("CI") == "true" && os.Getenv("GITHUB_WORKFLOW") != ""
}

// GetDefaultCache returns a default cache instance using the factory
func GetDefaultCache() Cache {
	factory := NewCacheFactory()
	return factory.CreateCache()
}

// GetCacheWithPrefix returns a cache instance with a custom prefix using the factory
func GetCacheWithPrefix(prefix string) Cache {
	factory := NewCacheFactory()
	return factory.CreateCacheWithPrefix(prefix)
}
