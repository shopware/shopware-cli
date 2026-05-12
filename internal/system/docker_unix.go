//go:build !windows

package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

func IsDockerMountable() bool {
	if runtime.GOOS == "darwin" {
		return true
	}

	if runtime.GOOS == "linux" {
		return os.Getuid() == 1000
	}

	return false
}

type dockerSettings struct {
	UseLibkrun bool `json:"UseLibkrun"`
}

// IsDockerUsingLibkrun checks if Docker is using libkrun (Docker VMM) for improved host mount performance on macOS.
func IsDockerUsingLibkrun() bool {
	if runtime.GOOS != "darwin" {
		return false
	}

	home := os.Getenv("HOME")
	if home == "" {
		return false
	}

	path := filepath.Join(home, "Library/Group Containers/group.com.docker/settings-store.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}

	var settings dockerSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return true
	}

	return settings.UseLibkrun
}
