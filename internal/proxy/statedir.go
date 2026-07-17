package proxy

import (
	"os"
	"path/filepath"
)

// StateDir returns the directory shopware-cli uses to keep proxy state: the
// project registry, the DNS daemon's PID file and its log. It is created if
// it does not exist yet.
func StateDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(configDir, "shopware-cli", "proxy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	return dir, nil
}
