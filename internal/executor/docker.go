package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// DockerExecutor runs commands via docker compose exec against the "web" service.
type DockerExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
}

func (d *DockerExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php", consoleCommandName(ctx))
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"php", consoleCommandName(ctx)}, args...))
}

func (d *DockerExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "composer")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"composer"}, args...))
}

func (d *DockerExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"php"}, args...))
}

func (d *DockerExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "npm")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"npm"}, args...))
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
	projectRootEnv := []string{"PROJECT_ROOT", "ADMIN_ROOT", "STOREFRONT_ROOT"}

	for _, k := range projectRootEnv {
		if _, ok := env[k]; ok {
			if strings.HasPrefix(env[k], d.projectRoot) {
				env[k] = d.NormalizePath(env[k])
			}
		}
	}

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

// newProcess wraps the command in a Process with a Docker-aware stop function.
// The innerArgs are the command args as seen inside the container (used as the
// pkill pattern for cleanup).
func (d *DockerExecutor) newProcess(cmd *exec.Cmd, innerArgs []string) *Process {
	projectRoot := d.projectRoot
	pattern := strings.Join(innerArgs, " ")

	return &Process{
		Cmd: cmd,
		stop: func(ctx context.Context) error {
			// First, kill the process inside the container via pkill.
			killCmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "web",
				"pkill", "-INT", "-f", pattern,
			)
			killCmd.Dir = projectRoot
			_ = killCmd.Run()

			// Also signal the host-side "docker compose exec" process so it
			// exits promptly instead of lingering after its child dies.
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGINT)
			}

			return nil
		},
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
