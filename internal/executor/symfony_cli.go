package executor

import (
	"context"
	"os/exec"
)

// SymfonyCLIExecutor runs commands through the Symfony CLI binary.
type SymfonyCLIExecutor struct {
	// Path to the symfony binary.
	BinaryPath string
	env        map[string]string
}

func (s *SymfonyCLIExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"php", consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.env, cmd)
	return cmd
}

func (s *SymfonyCLIExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"composer"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.env, cmd)
	return cmd
}

func (s *SymfonyCLIExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"php"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
	applyLocalEnv(s.env, cmd)
	return cmd
}

func (s *SymfonyCLIExecutor) Type() string {
	return "symfony-cli"
}

func (s *SymfonyCLIExecutor) WithEnv(env map[string]string) Executor {
	return &SymfonyCLIExecutor{BinaryPath: s.BinaryPath, env: env}
}
