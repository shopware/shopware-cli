package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

type DockerExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
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
	return TypeDocker
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

	return &DockerExecutor{env: mergeEnv(d.env, env), projectRoot: d.projectRoot, relDir: d.relDir, shopCfg: d.shopCfg, envCfg: d.envCfg}
}

func (d *DockerExecutor) WithRelDir(relDir string) Executor {
	return &DockerExecutor{env: d.env, projectRoot: d.projectRoot, relDir: relDir, shopCfg: d.shopCfg, envCfg: d.envCfg}
}

func (d *DockerExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, d.shopCfg, d.envCfg)
}

func (d *DockerExecutor) containerWorkdir() string {
	if d.relDir == "" {
		return "/var/www/html"
	}

	return filepath.Join("/var/www/html", d.relDir)
}

func (d *DockerExecutor) newProcess(cmd *exec.Cmd, innerArgs []string) *Process {
	projectRoot := d.projectRoot
	pattern := strings.Join(innerArgs, " ")

	return &Process{
		Cmd: cmd,
		stop: func(ctx context.Context) error {
			killCmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "web",
				"pkill", "-INT", "-f", pattern,
			)
			killCmd.Dir = projectRoot
			_ = killCmd.Run()

			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGINT)
			}

			return nil
		},
	}
}

func (d *DockerExecutor) StartEnvironment(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d")
	cmd.Dir = d.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

func (d *DockerExecutor) StopEnvironment(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "down")
	cmd.Dir = d.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

func (d *DockerExecutor) EnvironmentStatus(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--status=running", "-q")
	cmd.Dir = d.projectRoot

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking environment status: %w", err)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (d *DockerExecutor) baseArgs() []string {
	args := []string{"compose", "exec"}

	args = append(args, "-T")

	// When the web service runs as the mapped host user (see the compose
	// user: directive derived from system.ProjectUserSpec), that UID has no
	// passwd entry inside the image, so HOME is unset and tools like npm and
	// composer fall back to / and fail with EACCES. Point HOME at a writable
	// path, mirroring system.DockerRunUserArgs for the raw composer run.
	if system.ProjectUserSpec(d.projectRoot) != "" {
		args = append(args, "-e", "HOME=/tmp")
	}

	for k, v := range d.env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, "--workdir", d.containerWorkdir())

	args = append(args, "web")

	return args
}
