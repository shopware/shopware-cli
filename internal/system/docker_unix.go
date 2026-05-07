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

type Incompatibility struct {
	Title       string
	Description string
}

func CheckIncompatibilities(useDocker bool, projectFolder string) []Incompatibility {
	var incompatibilities []Incompatibility

	if useDocker && runtime.GOOS == "darwin" && !IsDockerUsingLibkrun() {
		incompatibilities = append(incompatibilities, Incompatibility{
			Title:       "Using Docker on macOS without libkrun (Docker VMM) may cause severe performance issues with file watching",
			Description: "Consider enabling libkrun in Docker Desktop settings for improved host mount performance",
		})
	}

	if IsWSL() && IsWSLWindowsMount(projectFolder) {
		incompatibilities = append(incompatibilities, Incompatibility{
			Title:       "Creating a project in a Windows-mounted directory (/mnt/c, etc.) under WSL is known to cause severe performance issues",
			Description: "Consider creating the project in the native Linux filesystem instead (e.g., ~/projects/)",
		})
	}

	return incompatibilities
}
