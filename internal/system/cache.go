package system

import (
	"os"
	"path"
)

// GetShopwareCliCacheDir returns the base cache directory for shopware-c
func GetShopwareCliCacheDir() string {
	if dir := os.Getenv("SHOPWARE_CLI_CACHE_DIR"); dir != "" {
		return dir
	}

	cacheDir, _ := os.UserCacheDir()

	return path.Join(cacheDir, "shopware-cli")
}
