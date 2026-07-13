package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/go-sql-driver/mysql"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
)

type LocalExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "php", cmdArgs...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	cmd := exec.CommandContext(ctx, "composer", args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	cmd := exec.CommandContext(ctx, "php", args...)
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

func (l *LocalExecutor) DatabaseConnection(ctx context.Context) (*mysql.Config, error) {
	return localDatabaseConnection(ctx, l.projectRoot)
}

func (l *LocalExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (l *LocalExecutor) StopEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (l *LocalExecutor) EnvironmentStatus(_ context.Context) (bool, error) {
	return false, ErrNotSupported
}

func applyLocalEnv(projectRoot string, env map[string]string, cmd *exec.Cmd) {
	cmd.Env = os.Environ()

	if projectRoot != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PROJECT_ROOT=%s", projectRoot))
	}

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}
