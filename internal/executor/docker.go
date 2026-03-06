package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mattn/go-isatty"
)

// DockerExecutor runs commands via docker compose exec against the "web" service.
type DockerExecutor struct{}

func (d *DockerExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs(ctx)
	dockerArgs = append(dockerArgs, "php", consoleCommandName(ctx))
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs(ctx)
	dockerArgs = append(dockerArgs, "composer")
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs(ctx)
	dockerArgs = append(dockerArgs, "php")
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) Type() string {
	return "docker"
}

func (d *DockerExecutor) baseArgs(ctx context.Context) []string {
	args := []string{"compose", "exec"}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		args = append(args, "-T")
	}

	for k, v := range getEnvVars(ctx) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, "web")

	return args
}
