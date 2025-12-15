package phpexec

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

type allowBinCIKey struct{}

func AllowBinCI(ctx context.Context) context.Context {
	return context.WithValue(ctx, allowBinCIKey{}, true)
}

var isCI = sync.OnceValue(func() bool {
	return os.Getenv("CI") != ""
})

var pathToSymfonyCLI = sync.OnceValue(func() string {
	path, err := exec.LookPath("symfony")
	if err != nil {
		return ""
	}
	return path
})

func symfonyCliAllowed() bool {
	return os.Getenv("SHOPWARE_CLI_NO_SYMFONY_CLI") != "1"
}

func ConsoleCommand(ctx context.Context, args ...string) *exec.Cmd {
	consoleCommand := "bin/console"

	if _, ok := ctx.Value(allowBinCIKey{}).(bool); ok && isCI() {
		consoleCommand = "bin/ci"
	}

	if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
		return exec.CommandContext(ctx, path, append([]string{"php", consoleCommand}, args...)...)
	}
	return exec.CommandContext(ctx, "php", append([]string{consoleCommand}, args...)...)
}

func ComposerCommand(ctx context.Context, args ...string) *exec.Cmd {
	if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
		return exec.CommandContext(ctx, path, append([]string{"composer"}, args...)...)
	}
	return exec.CommandContext(ctx, "composer", args...)
}

func PHPCommand(ctx context.Context, args ...string) *exec.Cmd {
	if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
		return exec.CommandContext(ctx, path, append([]string{"php"}, args...)...)
	}
	return exec.CommandContext(ctx, "php", args...)
}
