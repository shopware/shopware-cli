package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// LocalExecutor runs commands using the local PHP installation directly.
type LocalExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
}

func (l *LocalExecutor) ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := []string{consoleCommandName(ctx)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "php", cmdArgs...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "composer", args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "php", args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) NPMCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "npm", args...)
	applyLocalEnv(l.projectRoot, l.env, cmd)
	applyDir(resolveDir(l.projectRoot, l.relDir), cmd)
	logCmd(ctx, cmd)
	return cmd
}

func (l *LocalExecutor) NormalizePath(hostPath string) string {
	return hostPath
}

func (l *LocalExecutor) Type() string {
	return "local"
}

func (l *LocalExecutor) WithEnv(env map[string]string) Executor {
	return &LocalExecutor{env: env, projectRoot: l.projectRoot, relDir: l.relDir}
}

func (l *LocalExecutor) WithRelDir(relDir string) Executor {
	return &LocalExecutor{env: l.env, projectRoot: l.projectRoot, relDir: relDir}
}

// applyLocalEnv sets PROJECT_ROOT and extra environment variables on a local command.
func applyLocalEnv(projectRoot string, env map[string]string, cmd *exec.Cmd) {
	cmd.Env = os.Environ()

	if projectRoot != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PROJECT_ROOT=%s", projectRoot))
	}

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}
