package extension

import (
	"os"
	"path/filepath"
	"strings"
)

func PlatformPath(projectRoot, component, subPath string) string {
	return filepath.Join(projectRoot, PlatformRelPath(projectRoot, component, subPath))
}

// PlatformRelPath returns the platform component path relative to the project root.
func PlatformRelPath(projectRoot, component, subPath string) string {
	if _, err := os.Stat(filepath.Join(projectRoot, "src", "Core", "composer.json")); err == nil {
		return filepath.Join("src", component, subPath)
	} else if _, err := os.Stat(filepath.Join(projectRoot, "vendor", "shopware", "platform")); err == nil {
		return filepath.Join("vendor", "shopware", "platform", "src", component, subPath)
	}

	return filepath.Join("vendor", "shopware", strings.ToLower(component), subPath)
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
