package executor

import (
	"context"
	"os/exec"
)

// LocalExecutor runs commands using the local PHP installation directly.
type LocalExecutor struct{}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	return exec.CommandContext(ctx, "php", cmdArgs...)
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "composer", args...)
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "php", args...)
}

func (l *LocalExecutor) Type() string {
	return "local"
}
