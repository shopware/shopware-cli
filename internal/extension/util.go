package extension

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
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

// LoadSymfonyEnvFile loads the Symfony .env file from the project root.
func LoadSymfonyEnvFile(projectRoot string) error {
	currentEnv := os.Getenv("APP_ENV")
	if currentEnv == "" {
		currentEnv = "dev"
	}

	possibleEnvFiles := []string{
		path.Join(projectRoot, ".env.dist"),
		path.Join(projectRoot, ".env"),
		path.Join(projectRoot, ".env.local"),
		path.Join(projectRoot, ".env."+currentEnv),
		path.Join(projectRoot, ".env."+currentEnv+".local"),
	}

	var foundEnvFiles []string

	for _, envFile := range possibleEnvFiles {
		if _, err := os.Stat(envFile); err == nil {
			foundEnvFiles = append(foundEnvFiles, envFile)
		}
	}

	if len(foundEnvFiles) == 0 {
		return nil
	}

	currentMap, err := godotenv.Read(foundEnvFiles...)
	if err != nil {
		return err
	}

	for key, value := range currentMap {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}
