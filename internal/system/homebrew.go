package system

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsUnderHomebrew reports whether binaryPath is installed in Homebrew's bin directory.
func IsUnderHomebrew(binaryPath string) bool {
	brewExe, err := lookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.CommandContext(context.Background(), brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(binaryPath, brewBinPrefix)
}

// lookPath permits commands found through a relative PATH entry.
func lookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if errors.Is(err, exec.ErrDot) {
		return path, nil
	}
	return path, err
}
