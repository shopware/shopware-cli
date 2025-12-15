package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shyim/go-version"
)

// GetInstalledNodeVersion checks the installed Node.js version on the system.
func GetInstalledNodeVersion(ctx context.Context) (string, error) {
	// Check if Node.js is installed
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node.js is not installed: %w", err)
	}

	// Get the Node.js version
	cmd := exec.CommandContext(ctx, nodePath, "-v")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Node.js version: %w, output: %s", err, string(output))
	}

	// Node.js outputs version in format "vX.Y.Z", we need to trim the "v" prefix
	version := strings.TrimSpace(string(output))
	if len(version) > 0 && version[0] == 'v' {
		version = version[1:]
	}

	return version, nil
}

// IsNodeVersionAtLeast checks if the installed Node.js version meets the minimum required version.
func IsNodeVersionAtLeast(ctx context.Context, requiredVersion string) (bool, error) {
	installedVersion, err := GetInstalledNodeVersion(ctx)
	if err != nil {
		return false, err
	}

	nodeVersion, err := version.NewVersion(installedVersion)
	if err != nil {
		return false, fmt.Errorf("failed to parse installed Node.js version: %w", err)
	}

	constraint, err := version.NewConstraint(fmt.Sprintf(">= %s", requiredVersion))
	if err != nil {
		return false, fmt.Errorf("failed to parse required Node.js version constraint: %w", err)
	}

	return constraint.Check(nodeVersion), nil
}
