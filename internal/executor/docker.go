package executor

import (
	"context"
	"os"
	"os/exec"

	"github.com/mattn/go-isatty"
)

// DockerExecutor runs commands via docker compose exec against the "web" service.
type DockerExecutor struct{}

func (d *DockerExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "php", consoleCommandName(ctx))
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "composer")
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "php")
	dockerArgs = append(dockerArgs, args...)

	return exec.CommandContext(ctx, "docker", dockerArgs...)
}

func (d *DockerExecutor) Type() string {
	return "docker"
}

func (d *DockerExecutor) baseArgs() []string {
	args := []string{"compose", "exec"}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		args = append(args, "-T")
	}

	args = append(args, "web")

	return args
}
