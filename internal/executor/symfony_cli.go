package executor

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
)

type SymfonyCLIExecutor struct {
	BinaryPath  string
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

func (s *SymfonyCLIExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"php", consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	p := newProcess(exec.CommandContext(ctx, s.BinaryPath, cmdArgs...))
	applyLocalEnv(s.projectRoot, s.env, p.Cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), p.Cmd)
	logCmd(ctx, p.Cmd)
	return p
}

func (s *SymfonyCLIExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"composer"}
	cmdArgs = append(cmdArgs, args...)
	p := newProcess(exec.CommandContext(ctx, s.BinaryPath, cmdArgs...))
	applyLocalEnv(s.projectRoot, s.env, p.Cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), p.Cmd)
	logCmd(ctx, p.Cmd)
	return p
}

func (s *SymfonyCLIExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"php"}
	cmdArgs = append(cmdArgs, args...)
	p := newProcess(exec.CommandContext(ctx, s.BinaryPath, cmdArgs...))
	applyLocalEnv(s.projectRoot, s.env, p.Cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), p.Cmd)
	logCmd(ctx, p.Cmd)
	return p
}

func (s *SymfonyCLIExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	p := newProcess(exec.CommandContext(ctx, "npm", args...))
	applyLocalEnv(s.projectRoot, s.env, p.Cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), p.Cmd)
	logCmd(ctx, p.Cmd)
	return p
}

func (s *SymfonyCLIExecutor) AvailableLogFiles(_ context.Context) ([]LogFile, error) {
	return localLogFiles(filepath.Join(s.projectRoot, "var", "log"))
}

func (s *SymfonyCLIExecutor) GetLog(ctx context.Context, file string, lines int, follow bool, w io.Writer) error {
	p := newProcess(exec.CommandContext(ctx, "tail", tailArgs(logFilePath(s.projectRoot, file), lines, follow)...))
	applyLocalEnv(s.projectRoot, s.env, p.Cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), p.Cmd)
	logCmd(ctx, p.Cmd)

	return runStreaming(ctx, p.Cmd, w)
}

func (s *SymfonyCLIExecutor) NormalizePath(hostPath string) string {
	return hostPath
}

func (s *SymfonyCLIExecutor) Type() string {
	return TypeSymfonyCLI
}

func (s *SymfonyCLIExecutor) WithEnv(env map[string]string) Executor {
	return &SymfonyCLIExecutor{BinaryPath: s.BinaryPath, env: mergeEnv(s.env, env), projectRoot: s.projectRoot, relDir: s.relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SymfonyCLIExecutor) WithRelDir(relDir string) Executor {
	return &SymfonyCLIExecutor{BinaryPath: s.BinaryPath, env: s.env, projectRoot: s.projectRoot, relDir: relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SymfonyCLIExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, s.shopCfg, s.envCfg)
}

func (s *SymfonyCLIExecutor) ShopConfig() *shop.Config {
	return s.shopCfg
}

func (s *SymfonyCLIExecutor) DatabaseConnection(_ context.Context) (*DatabaseConnection, error) {
	return databaseConnectionFromEnv(s.projectRoot, s.env)
}

func (s *SymfonyCLIExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SymfonyCLIExecutor) StopEnvironment(_ context.Context, _ StopOptions) error {
	return ErrNotSupported
}

func (s *SymfonyCLIExecutor) EnvironmentStatus(_ context.Context) (bool, error) {
	return false, ErrNotSupported
}
