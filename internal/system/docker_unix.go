//go:build !windows

package system

import (
	"os"
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
