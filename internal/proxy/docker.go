package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runDocker executes the docker CLI and returns its combined output.
func runDocker(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker is required for the proxy but was not found in PATH")
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}
