package devtui

import (
	"context"
	"os/exec"
)

// composeCommand builds a `docker compose <args...>` command rooted at
// projectRoot. It is the single place the devtui package shells out to docker
// compose, so the binary name and working directory are defined once.
func composeCommand(ctx context.Context, projectRoot string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = projectRoot
	return cmd
}
