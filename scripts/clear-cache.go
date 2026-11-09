//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/system"
)

func main() {
	if err := clearCache(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func clearCache() error {
	cacheDir := filepath.Clean(system.GetShopwareCliCacheDir())
	if !filepath.IsAbs(cacheDir) || filepath.Dir(cacheDir) == cacheDir {
		return fmt.Errorf("cannot clear cache directory %q: must be an absolute path other than the filesystem root", cacheDir)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("cannot clear cache directory: %w", err)
	}
	fmt.Printf("Cleared shopware-cli cache: %s\n", cacheDir)
	return nil
}
