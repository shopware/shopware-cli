package executor

import (
	"context"
	"os/exec"
)

// SymfonyCLIExecutor runs commands through the Symfony CLI binary.
type SymfonyCLIExecutor struct {
	// Path to the symfony binary.
	BinaryPath string
}

func (s *SymfonyCLIExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"php", consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	return exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
}

func (s *SymfonyCLIExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"composer"}
	cmdArgs = append(cmdArgs, args...)
	return exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
}

func (s *SymfonyCLIExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{"php"}
	cmdArgs = append(cmdArgs, args...)
	return exec.CommandContext(ctx, s.BinaryPath, cmdArgs...)
}

func (s *SymfonyCLIExecutor) Type() string {
	return "symfony-cli"
}
