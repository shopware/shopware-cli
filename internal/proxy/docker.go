package proxy

import (
	"context"
	"fmt"
	"os/exec"
)

// runDocker runs `docker <args...>` and returns its combined output.
func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %v: %w\n%s", args, err, out)
	}

	return string(out), nil
}
