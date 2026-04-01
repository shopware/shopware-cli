package executor

import (
	"context"
	"os/exec"
)

// SymfonyCLIExecutor runs commands through the Symfony CLI binary.
type SymfonyCLIExecutor struct {
	BinaryPath  string
	env         map[string]string
	projectRoot string
	relDir      string
}

func (s *SymfonyCLIExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"php", consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.projectRoot, s.env, cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (s *SymfonyCLIExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"composer"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.projectRoot, s.env, cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (s *SymfonyCLIExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	cmdArgs := []string{"php"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.projectRoot, s.env, cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (s *SymfonyCLIExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	cmd := exec.CommandContext(ctx, "npm", args...)
	applyLocalEnv(s.projectRoot, s.env, cmd)
	applyDir(resolveDir(s.projectRoot, s.relDir), cmd)
	logCmd(ctx, cmd)
	return newProcess(cmd)
}

func (s *SymfonyCLIExecutor) NormalizePath(hostPath string) string {
	return hostPath
}

func (s *SymfonyCLIExecutor) Type() string {
	return TypeSymfonyCLI
}

func (s *SymfonyCLIExecutor) WithEnv(env map[string]string) Executor {
	return &SymfonyCLIExecutor{BinaryPath: s.BinaryPath, env: mergeEnv(s.env, env), projectRoot: s.projectRoot, relDir: s.relDir}
}

func (s *SymfonyCLIExecutor) WithRelDir(relDir string) Executor {
	return &SymfonyCLIExecutor{BinaryPath: s.BinaryPath, env: s.env, projectRoot: s.projectRoot, relDir: relDir}
}

func (s *SymfonyCLIExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SymfonyCLIExecutor) StopEnvironment(_ context.Context) error {
	return ErrNotSupported
}
