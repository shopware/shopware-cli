//go:build !windows

package system

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func IsDockerMountable() bool {
	if runtime.GOOS == "darwin" {
		return true
	}

	// On Linux we always pass --user with the host UID/GID (see
	// DockerRunUserArgs), so bind mounts are writable for any host user,
	// not only UID 1000.
	return runtime.GOOS == "linux"
}

// ProjectUserSpec returns the "uid:gid" the container should run as for
// the given project directory: the calling user's UID combined with the
// directory's owning group. Using the directory's group makes shared
// setups work (e.g. a projects dir owned by a dev group with the setgid
// bit): files created by the container belong to the shared group, so
// other members can collaborate. Falls back to the caller's primary GID
// when the directory cannot be inspected. Returns "" on non-Linux
// platforms: only there do bind mounts expose raw host UIDs; Docker
// Desktop's VM handles ownership mapping on macOS and Windows.
func ProjectUserSpec(dir string) string {
	if runtime.GOOS != "linux" {
		return ""
	}

	uid := os.Getuid()
	gid := os.Getgid()
	if uid < 0 || gid < 0 {
		return ""
	}

	if fi, err := os.Stat(dir); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			gid = int(st.Gid)
		}
	}

	return fmt.Sprintf("%d:%d", uid, gid)
}

// DockerRunUserArgs returns the arguments for a raw `docker run` so that
// the container process runs as the host user with the project
// directory's group. An arbitrary UID has no passwd entry inside the
// image, which would leave HOME unset/unwritable, so HOME and
// COMPOSER_HOME are pointed at a writable location.
func DockerRunUserArgs(projectDir string) []string {
	spec := ProjectUserSpec(projectDir)
	if spec == "" {
		return nil
	}
	return []string{
		"--user", spec,
		"-e", "HOME=/tmp",
		"-e", "COMPOSER_HOME=/tmp/composer",
	}
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
