package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/shyim/go-version"
)

// phpVersionProbeTimeout bounds how long a single candidate executable may
// take to report its version before it is considered broken.
const phpVersionProbeTimeout = 10 * time.Second

// resolvePHPBinary returns the PHP binary to use. It prefers the PHP_BINARY
// environment variable, matching the convention runComposerInstall already
// follows, and falls back to the "php" binary found in PATH.
func resolvePHPBinary() (string, error) {
	if phpBinary := os.Getenv("PHP_BINARY"); phpBinary != "" {
		return phpBinary, nil
	}

	return exec.LookPath("php")
}

// phpVersionOutput extracts the normalized version from the version banner in
// `php -v` output, e.g. "PHP 8.3.6-1ubuntu1 (cli) ..." -> "8.3.6". (?m) is
// required: PHP prints startup warnings before the banner, so anchoring to the
// start of the whole output would reject a usable PHP.
var phpVersionOutput = regexp.MustCompile(`(?m)^PHP\s+(\d+\.\d+(?:\.\d+)?)`)

// GetPHPVersionOfBinary executes the given PHP binary and returns the
// normalized version it reports. It fails when the binary cannot be executed
// or does not produce PHP's version banner.
func GetPHPVersionOfBinary(ctx context.Context, phpPath string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, phpVersionProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, phpPath, "-v")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get PHP version: %w, output: %s", err, string(output))
	}

	matches := phpVersionOutput.FindStringSubmatch(strings.TrimSpace(string(output)))
	if matches == nil {
		return "", fmt.Errorf("unexpected output format: %s", string(output))
	}

	return matches[1], nil
}

// GetInstalledPHPVersion checks the installed PHP version on the system.
func GetInstalledPHPVersion(ctx context.Context) (string, error) {
	// Check if PHP is installed
	phpPath, err := resolvePHPBinary()
	if err != nil {
		return "", fmt.Errorf("PHP is not installed: %w", err)
	}

	return GetPHPVersionOfBinary(ctx, phpPath)
}

// GetAvailablePHPExtensions returns the list of loaded PHP extensions by parsing `php -m` output.
func GetAvailablePHPExtensions(ctx context.Context) ([]string, error) {
	phpPath, err := resolvePHPBinary()
	if err != nil {
		return nil, fmt.Errorf("PHP is not installed: %w", err)
	}

	cmd := exec.CommandContext(ctx, phpPath, "-m")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP extensions: %w, output: %s", err, string(output))
	}

	var extensions []string
	for line := range strings.SplitSeq(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		extensions = append(extensions, line)
	}

	return extensions, nil
}

func IsPHPVersionAtLeast(ctx context.Context, requiredVersion string) (bool, error) {
	installedVersion, err := GetInstalledPHPVersion(ctx)
	if err != nil {
		return false, err
	}

	return phpVersionAtLeast(installedVersion, requiredVersion), nil
}

// phpVersionAtLeast reports whether installedVersion is at least
// requiredVersion. Unparseable versions report false.
func phpVersionAtLeast(installedVersion, requiredVersion string) bool {
	phpVersion, err := version.NewVersion(installedVersion)
	if err != nil {
		return false
	}

	constraint, err := version.NewConstraint(">= " + requiredVersion)
	if err != nil {
		return false
	}

	return constraint.Check(phpVersion)
}
