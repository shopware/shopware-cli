package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// containerStatus returns the Docker status of the named container, such as
// "running", "restarting" or "exited", and how often its restart policy has
// restarted it. Both are zero when the container does not exist.
func containerStatus(ctx context.Context, name string) (string, int) {
	out, err := runDocker(ctx, "inspect", "--format", "{{.State.Status}} {{.RestartCount}}", name)
	if err != nil {
		return "", 0
	}

	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", 0
	}

	restarts, _ := strconv.Atoi(fields[1])

	return fields[0], restarts
}

// containerIdle reports whether the container is absent or stopped cleanly.
func containerIdle(ctx context.Context, name string) bool {
	status, _ := containerStatus(ctx, name)

	return idleState(status)
}

// containerCrashLooping reports whether the container keeps restarting.
func containerCrashLooping(ctx context.Context, name string) bool {
	return crashLooping(containerStatus(ctx, name))
}

// idleState tells a clean stop from a broken container: restarting, paused
// and dead containers are not idle.
func idleState(status string) bool {
	switch status {
	case "", "created", "exited":
		return true
	default:
		return false
	}
}

// crashLooping is true while a container restarts, and also in the moments
// between restarts, when it shows as running with restarts already counted.
func crashLooping(status string, restarts int) bool {
	return status == "restarting" || (status == "running" && restarts > 0)
}
