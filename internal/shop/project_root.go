package shop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindClosestShopwareProject walks from the current directory towards the
// filesystem root until it finds a Shopware project. When allowFallback is
// true, it returns the current directory if no project is found.
func FindClosestShopwareProject(allowFallback bool) (string, error) {
	if projectRoot := os.Getenv("PROJECT_ROOT"); projectRoot != "" {
		// PROJECT_ROOT is an explicit override used by local tooling and tests.
		// Keep it authoritative even when the project is only partially set up.
		return filepath.Clean(projectRoot), nil
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	startDir := currentDir

	for {
		isProject, err := isShopwareProject(currentDir)
		if err != nil {
			return "", err
		}
		if isProject {
			return currentDir, nil
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	if allowFallback {
		return startDir, nil
	}

	return "", errors.New("cannot find Shopware project in current directory")
}

func isShopwareProject(path string) (bool, error) {
	if info, err := os.Stat(filepath.Join(path, "bin", "console")); err != nil || info.IsDir() {
		return false, nil
	}

	for _, name := range []string{"composer.json", "composer.lock"} {
		content, err := os.ReadFile(filepath.Join(path, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", filepath.Join(path, name), err)
		}
		if strings.Contains(string(content), "shopware/core") {
			return true, nil
		}
	}

	return false, nil
}
