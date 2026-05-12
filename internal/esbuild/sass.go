package esbuild

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

const dartSassVersion = "1.69.7"

//go:embed static/variables.scss
var scssVariables []byte

//go:embed static/mixins.scss
var scssMixins []byte

// IsDartSassAvailable checks whether dart-sass is available locally (on PATH or in cache),
// without triggering a network download. Returns true if the binary is available.
func IsDartSassAvailable() bool {
	if _, err := exec.LookPath("dart-sass"); err == nil {
		return true
	}

	cache := system.GetCacheWithPrefix("dart-sass")
	cacheKey := "dart-sass-" + dartSassVersion + "-" + runtime.GOOS + "-" + runtime.GOARCH

	expectedBinary := "sass"
	if runtime.GOOS == "windows" {
		expectedBinary += ".bat"
	}

	cachedPath, err := cache.GetFolderCachePath(context.Background(), cacheKey)
	if err != nil {
		return false
	}

	executablePath := filepath.Join(cachedPath, expectedBinary)
	if _, statErr := os.Stat(executablePath); statErr == nil {
		return true
	}

	return false
}

func locateDartSass(ctx context.Context) (string, error) {
	if exePath, err := exec.LookPath("dart-sass"); err == nil {
		return exePath, nil
	}

	// Create cache instance for dart-sass
	cache := system.GetCacheWithPrefix("dart-sass")

	cacheKey := "dart-sass-" + dartSassVersion + "-" + runtime.GOOS + "-" + runtime.GOARCH

	expectedBinary := "sass"
	//goland:noinspection ALL
	if runtime.GOOS == "windows" {
		expectedBinary += ".bat"
	}

	// Try to get the cached folder
	cachedPath, err := cache.GetFolderCachePath(ctx, cacheKey)
	if err == nil {
		// Cache hit - return the path to the executable
		executablePath := filepath.Join(cachedPath, expectedBinary)
		if _, statErr := os.Stat(executablePath); statErr == nil {
			return executablePath, nil
		}
	} else if err != system.ErrCacheNotFound {
		return "", fmt.Errorf("cache error: %w", err)
	}

	// Cache miss - need to download and extract
	logging.FromContext(ctx).Infof("Downloading dart-sass")

	// Create a temporary directory for download
	downloadDir, err := os.MkdirTemp("", "dart-sass-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(downloadDir)
	}()

	if err := downloadDartSass(ctx, downloadDir); err != nil {
		return "", err
	}

	// Store the downloaded folder in cache
	if err := cache.StoreFolderCache(ctx, cacheKey, downloadDir); err != nil {
		logging.FromContext(ctx).Debugf("cannot cache dart-sass folder: %v", err)
	}

	// Create a permanent directory for the restored cache
	permanentDir, err := os.MkdirTemp("", "dart-sass-permanent-*")
	if err != nil {
		return "", fmt.Errorf("failed to create permanent directory: %w", err)
	}

	// Restore cached folder to permanent location
	if err := cache.RestoreFolderCache(ctx, cacheKey, permanentDir); err != nil {
		_ = os.RemoveAll(permanentDir)
		return "", fmt.Errorf("failed to restore cached folder: %w", err)
	}

	return filepath.Join(permanentDir, expectedBinary), nil
}
