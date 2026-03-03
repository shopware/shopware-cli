package executor

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

// Executor abstracts command execution across different environment types.
type Executor interface {
	// ConsoleCommand returns an exec.Cmd for running bin/console.
	ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd

	// ComposerCommand returns an exec.Cmd for running composer.
	ComposerCommand(ctx context.Context, args ...string) *exec.Cmd

	// PHPCommand returns an exec.Cmd for running php.
	PHPCommand(ctx context.Context, args ...string) *exec.Cmd

	// Type returns the executor type name (e.g. "local", "docker").
	Type() string
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
