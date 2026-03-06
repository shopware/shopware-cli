package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// LocalExecutor runs commands using the local PHP installation directly.
type LocalExecutor struct{}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "php", cmdArgs...)
	applyLocalEnv(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "composer", args...)
	applyLocalEnv(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "php", args...)
	applyLocalEnv(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) Type() string {
	return "local"
}

// applyLocalEnv sets extra environment variables from the context on a local command.
func applyLocalEnv(ctx context.Context, cmd *exec.Cmd) {
	env := getEnvVars(ctx)
	if len(env) == 0 {
		return
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}
