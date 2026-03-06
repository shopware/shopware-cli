package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// LocalExecutor runs commands using the local PHP installation directly.
type LocalExecutor struct {
	env map[string]string
}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "php", cmdArgs...)
	applyLocalEnv(l.env, cmd)
	return cmd
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "composer", args...)
	applyLocalEnv(l.env, cmd)
	return cmd
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "php", args...)
	applyLocalEnv(l.env, cmd)
	return cmd
}

func (l *LocalExecutor) Type() string {
	return "local"
}

func (l *LocalExecutor) WithEnv(env map[string]string) Executor {
	return &LocalExecutor{env: env}
}

// applyLocalEnv sets extra environment variables on a local command.
func applyLocalEnv(env map[string]string, cmd *exec.Cmd) {
	if len(env) == 0 {
		return
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}
