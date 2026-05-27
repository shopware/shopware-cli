package packagist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstalledPackage is a single entry of vendor/composer/installed.json.
type InstalledPackage struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Require     map[string]string `json:"require"`
	InstallPath string            `json:"install-path"`
}

// InstalledJson models vendor/composer/installed.json, composer's record of
// every package actually installed into the project.
type InstalledJson struct {
	Packages []InstalledPackage `json:"packages"`
}

// ReadInstalledJson reads vendor/composer/installed.json relative to
// projectRoot. An empty InstalledJson is returned when the file does not
// exist, so callers can treat "no composer install yet" as "no packages".
func ReadInstalledJson(projectRoot string) (*InstalledJson, error) {
	installedPath := filepath.Join(projectRoot, "vendor", "composer", "installed.json")

	content, err := os.ReadFile(installedPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &InstalledJson{}, nil
		}
		return nil, fmt.Errorf("read installed.json: %w", err)
	}

	var installed InstalledJson
	if err := json.Unmarshal(content, &installed); err != nil {
		return nil, fmt.Errorf("parse installed.json: %w", err)
	}

	return &installed, nil
}

// InstallDirName resolves the package's install location (install-path is
// recorded relative to vendor/composer) and, when it sits directly inside
// baseDir, returns the child directory name. ok is false when the package is
// installed anywhere else. Symlinks are resolved on both sides so symlinked
// packages still match.
func (p InstalledPackage) InstallDirName(projectRoot, baseDir string) (string, bool) {
	if p.InstallPath == "" {
		return "", false
	}

	abs := p.InstallPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, "vendor", "composer", p.InstallPath)
	}

	rel, err := filepath.Rel(resolveSymlinks(baseDir), resolveSymlinks(abs))
	if err != nil {
		return "", false
	}

	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}

	if strings.ContainsRune(rel, filepath.Separator) {
		return "", false
	}

	return rel, true
}

func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}
