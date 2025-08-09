package system

import (
	"os"
	"path"
)

// GetShopwareCliCacheDir returns the base cache directory for shopware-c
func GetShopwareCliCacheDir() string {
	cacheDir, _ := os.UserCacheDir()

	return path.Join(cacheDir, "shopware-cli")
}
