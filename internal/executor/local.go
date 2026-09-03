package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

type LocalExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

// resolveProjectPHPBinary is a seam for tests, which must not depend on the PHP
// versions installed on the machine running them.
var resolveProjectPHPBinary = system.ResolveProjectPHPBinary

// resolveComposer is a seam for tests, which must not depend on a Composer
// installation or trigger a real PHAR download.
var resolveComposer = system.ResolveComposer

// phpBinary returns the PHP executable used for this project's commands,
// following the precedence: PHP_BINARY > php_version from .shopware-project.yml >
// "php" from PATH. Resolution failures are returned rather than falling back, so
// the project never silently runs on a different PHP version.
func (l *LocalExecutor) phpBinary(ctx context.Context) (string, error) {
	var pin string
	if l.shopCfg != nil {
		pin = l.shopCfg.PHPVersion
	}

	phpBinary, err := resolveProjectPHPBinary(ctx, pin)
	if err != nil {
		return "", err
	}
	if phpBinary != "" {
		return phpBinary, nil
	}

	return "php", nil
}

// phpCommand builds a command running the project's PHP. A resolution failure is
// attached to the command so it surfaces from Run/Output.
func (l *LocalExecutor) phpCommand(ctx context.Context, args ...string) *exec.Cmd {
	phpBinary, err := l.phpBinary(ctx)
	if err != nil {
		cmd := exec.CommandContext(ctx, "php", args...)
		cmd.Err = err
		return cmd
	}

	return exec.CommandContext(ctx, phpBinary, args...)
}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := l.phpCommand(ctx, cmdArgs...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	var cmd *exec.Cmd

	phpBinary, err := l.phpBinary(ctx)
	if err != nil {
		cmd = exec.CommandContext(ctx, "composer", args...)
		cmd.Err = err
		return newProcess(cmd)
	}

	composerBinary, isPhar, err := resolveComposer(ctx)
	if err != nil {
		cmd = exec.CommandContext(ctx, "composer", args...)
		cmd.Err = err
		return newProcess(cmd)
	}

	// Run Composer through PHP when a specific PHP executable is selected (so
	// dependency resolution and scripts use the same PHP as the project) or
	// when only the downloaded PHAR is available.
	if isPhar || phpBinary != "php" {
		cmd = exec.CommandContext(ctx, phpBinary, append([]string{composerBinary}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, "composer", args...)
	}

	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	cmd := l.phpCommand(ctx, args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	cmd := exec.CommandContext(ctx, "npm", args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) Command(ctx context.Context, name string, args ...string) *Process {
	cmd := exec.CommandContext(ctx, name, args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) NormalizePath(hostPath string) string {
	return hostPath
}

func (l *LocalExecutor) Type() string {
	return TypeLocal
}

func (l *LocalExecutor) WithEnv(env map[string]string) Executor {
	return &LocalExecutor{env: mergeEnv(l.env, env), projectRoot: l.projectRoot, relDir: l.relDir, shopCfg: l.shopCfg, envCfg: l.envCfg}
}

func (l *LocalExecutor) WithRelDir(relDir string) Executor {
	return &LocalExecutor{env: l.env, projectRoot: l.projectRoot, relDir: relDir, shopCfg: l.shopCfg, envCfg: l.envCfg}
}

func (l *LocalExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, l.shopCfg, l.envCfg)
}

func (l *LocalExecutor) ShopConfig() *shop.Config {
	return l.shopCfg
}

func (l *LocalExecutor) DatabaseConnection(_ context.Context) (*DatabaseConnection, error) {
	return databaseConnectionFromEnv(l.projectRoot, l.env)
}

func (l *LocalExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (l *LocalExecutor) StopEnvironment(_ context.Context, _ StopOptions) error {
	return ErrNotSupported
}

func (l *LocalExecutor) EnvironmentStatus(_ context.Context) (bool, error) {
	return false, ErrNotSupported
}

func applyLocalEnv(projectRoot string, env map[string]string, cmd *exec.Cmd) {
	cmd.Env = os.Environ()

	if projectRoot != "" {
		cmd.Env = append(cmd.Env, "PROJECT_ROOT="+projectRoot)
	}

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}
