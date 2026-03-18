package extension

import (
	"os"
	"path/filepath"
	"strings"
)

func PlatformPath(projectRoot, component, path string) string {
	if _, err := os.Stat(filepath.Join(projectRoot, "src", "Core", "composer.json")); err == nil {
		return filepath.Join(projectRoot, "src", component, path)
	} else if _, err := os.Stat(filepath.Join(projectRoot, "vendor", "shopware", "platform")); err == nil {
		return filepath.Join(projectRoot, "vendor", "shopware", "platform", "src", component, path)
	}

	return filepath.Join(projectRoot, "vendor", "shopware", strings.ToLower(component), path)
}

// projectRequiresBuild checks if the project is a contribution project aka shopware/shopware.
func projectRequiresBuild(projectRoot string) bool {
	// We work inside Shopware itself
	if _, err := os.Stat(filepath.Join(projectRoot, "src", "Core", "composer.json")); err == nil {
		return true
	}

	// vendor/shopware/platform does never have assets pre-build
	if _, err := os.Stat(filepath.Join(projectRoot, "vendor", "shopware", "platform", "composer.json")); err == nil {
		return true
	}

	return false
}
