package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// EnsureComposeSupportsReset verifies docker compose is at least 2.24, the
// first version supporting the !reset YAML tag the proxy's compose override
// relies on to clear the base file's fixed host ports.
func EnsureComposeSupportsReset(ctx context.Context) error {
	out, err := runDocker(ctx, "compose", "version", "--short")
	if err != nil {
		return err
	}

	version := strings.TrimPrefix(strings.TrimSpace(out), "v")
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return nil // unparseable, assume recent enough
	}

	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return nil //nolint:nilerr // unparseable, assume recent enough
	}

	if major > 2 || (major == 2 && minor >= 24) {
		return nil
	}

	return fmt.Errorf("docker compose %s is too old for the shared proxy, version 2.24 or newer is required", version)
}

// runDocker runs `docker <args...>` and returns its combined output.
func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %v: %w\n%s", args, err, out)
	}

	return string(out), nil
}
