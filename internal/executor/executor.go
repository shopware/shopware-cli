package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/logging"
)

// ErrNotSupported is returned when the executor does not support a managed environment.
var ErrNotSupported = errors.New("operation not supported by this executor")

// Executor type constants returned by Executor.Type().
const (
	TypeDocker     = "docker"
	TypeLocal      = "local"
	TypeSymfonyCLI = "symfony-cli"
)

// Executor abstracts command execution across different environment types.
type Executor interface {
	ConsoleCommand(ctx context.Context, args ...string) *Process
	ComposerCommand(ctx context.Context, args ...string) *Process
	PHPCommand(ctx context.Context, args ...string) *Process
	NPMCommand(ctx context.Context, args ...string) *Process
	// NormalizePath converts a host-absolute path to the path seen by the execution
	// environment. For local executors the path is returned unchanged; for Docker it
	// is translated to the container mount (e.g. /var/www/html/...).
	NormalizePath(hostPath string) string
	Type() string
	WithEnv(env map[string]string) Executor
	WithRelDir(relDir string) Executor
	// StartEnvironment starts the backing environment (e.g. docker compose up -d).
	// Returns ErrNotSupported for executors that have no managed environment.
	StartEnvironment(ctx context.Context) error
	// StopEnvironment stops the backing environment (e.g. docker compose down).
	// Returns ErrNotSupported for executors that have no managed environment.
	StopEnvironment(ctx context.Context) error
}

type allowBinCIKey struct{}

// AllowBinCI marks a context so that ConsoleCommand may use bin/ci instead of bin/console in CI environments.
func AllowBinCI(ctx context.Context) context.Context {
	return context.WithValue(ctx, allowBinCIKey{}, true)
}

// IsBinCIAllowed returns true if the context has AllowBinCI set and the CI env var is detected.
func IsBinCIAllowed(ctx context.Context) bool {
	_, ok := ctx.Value(allowBinCIKey{}).(bool)
	return ok && isCI()
}

var isCI = sync.OnceValue(func() bool {
	return os.Getenv("CI") != ""
})

// consoleCommandName returns "bin/ci" or "bin/console" depending on context and CI detection.
func consoleCommandName(ctx context.Context) string {
	if IsBinCIAllowed(ctx) {
		return "bin/ci"
	}
	return "bin/console"
}

// resolveDir returns the absolute directory from projectRoot and relDir.
func resolveDir(projectRoot, relDir string) string {
	if relDir == "" {
		return projectRoot
	}

	return filepath.Join(projectRoot, relDir)
}

// applyDir sets the working directory on a command if dir is non-empty.
func applyDir(dir string, cmd *exec.Cmd) {
	if dir != "" {
		cmd.Dir = dir
	}
}

// logCmd logs the command that will be executed at debug level.
func logCmd(ctx context.Context, cmd *exec.Cmd) {
	logging.FromContext(ctx).Debugf("exec: %s (dir: %s)", strings.Join(cmd.Args, " "), cmd.Dir)
}

func mergeEnv(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}
