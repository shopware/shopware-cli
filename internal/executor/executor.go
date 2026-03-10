package executor

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

// Executor abstracts command execution across different environment types.
type Executor interface {
	ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd
	ComposerCommand(ctx context.Context, args ...string) *exec.Cmd
	PHPCommand(ctx context.Context, args ...string) *exec.Cmd
	Type() string
	WithEnv(env map[string]string) Executor
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
