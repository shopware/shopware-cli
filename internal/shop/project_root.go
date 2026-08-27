package shop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func FindClosestShopwareProject() (string, error) {
	projectRoot := os.Getenv("PROJECT_ROOT")

	if projectRoot != "" {
		return projectRoot, nil
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		files := []string{
			currentDir + "/composer.json",
			currentDir + "/composer.lock",
		}

		for _, file := range files {
			if _, err := os.Stat(file); err == nil {
				content, err := os.ReadFile(file)
				if err != nil {
					return "", err
				}
				contentString := string(content)

				if strings.Contains(contentString, "shopware/core") {
					if _, err := os.Stat(currentDir + "/bin/console"); err == nil {
						return currentDir, nil
					}
				}
			}
		}

		currentDir = filepath.Dir(currentDir)

		if currentDir == filepath.Dir(currentDir) {
			break
		}
	}

	return "", errors.New("cannot find Shopware project in current directory")
}
