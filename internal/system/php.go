package system

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shyim/go-version"
)

// GetInstalledPHPVersion checks the installed PHP version on the system.
func GetInstalledPHPVersion(ctx context.Context) (string, error) {
	// Check if PHP is installed
	phpPath, err := exec.LookPath("php")
	if err != nil {
		return "", fmt.Errorf("PHP is not installed: %w", err)
	}

	// Get the PHP version
	cmd := exec.CommandContext(ctx, phpPath, "-v")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get PHP version: %w, output: %s", err, string(output))
	}

	splitt := strings.Split(string(output), " ")

	if len(splitt) < 2 {
		return "", fmt.Errorf("unexpected output format: %s", string(output))
	}

	// Parse the version from the output
	version := splitt[1]
	return strings.TrimSpace(version), nil
}

func IsPHPVersionAtLeast(ctx context.Context, requiredVersion string) (bool, error) {
	installedVersion, err := GetInstalledPHPVersion(ctx)
	if err != nil {
		return false, err
	}

	phpVersion, err := version.NewVersion(installedVersion)
	if err != nil {
		return false, fmt.Errorf("failed to parse installed PHP version: %w", err)
	}

	constraint, err := version.NewConstraint(fmt.Sprintf(">= %s", requiredVersion))
	if err != nil {
		return false, fmt.Errorf("failed to parse required PHP version constraint: %w", err)
	}

	return constraint.Check(phpVersion), nil
}
