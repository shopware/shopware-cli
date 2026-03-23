package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

// DockerExecutor runs commands via docker compose exec against the "web" service.
type DockerExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
}

func (d *DockerExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php", consoleCommandName(ctx))
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	cancelWithSIGINT(cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (d *DockerExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "composer")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	cancelWithSIGINT(cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (d *DockerExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	cancelWithSIGINT(cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (d *DockerExecutor) NPMCommand(ctx context.Context, args ...string) *exec.Cmd {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "npm")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	cancelWithSIGINT(cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (d *DockerExecutor) NormalizePath(hostPath string) string {
	if d.projectRoot == "" {
		return hostPath
	}

	rel, err := filepath.Rel(d.projectRoot, hostPath)
	if err != nil {
		return hostPath
	}

	return filepath.Join("/var/www/html", rel)
}

func (d *DockerExecutor) Type() string {
	return "docker"
}

func (d *DockerExecutor) WithEnv(env map[string]string) Executor {
	return &DockerExecutor{env: mergeEnv(d.env, env), projectRoot: d.projectRoot, relDir: d.relDir}
}

func (d *DockerExecutor) WithRelDir(relDir string) Executor {
	return &DockerExecutor{env: d.env, projectRoot: d.projectRoot, relDir: relDir}
}

// containerWorkdir returns the container-side working directory.
func (d *DockerExecutor) containerWorkdir() string {
	if d.relDir == "" {
		return "/var/www/html"
	}

	return filepath.Join("/var/www/html", d.relDir)
}

func cancelWithSIGINT(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGINT)
	}
}

func (d *DockerExecutor) baseArgs() []string {
	args := []string{"compose", "exec"}

	args = append(args, "-T")

	for k, v := range d.env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, "--workdir", d.containerWorkdir())

	args = append(args, "web")

	return args
}
