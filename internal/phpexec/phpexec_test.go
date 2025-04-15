package phpexec

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSymfonyDetection(t *testing.T) {
	testCases := []struct {
		Name        string
		Func        func(context.Context, ...string) *exec.Cmd
		Args        []string
		SymfonyArgs []string
	}{
		{
			Name:        "Composer",
			Func:        ComposerCommand,
			Args:        []string{"composer"},
			SymfonyArgs: []string{"/test/symfony", "composer"},
		},
		{
			Name:        "Console",
			Func:        ConsoleCommand,
			Args:        []string{"php", "bin/console"},
			SymfonyArgs: []string{"/test/symfony", "php", "bin/console"},
		},
		{
			Name:        "PHP",
			Func:        PHPCommand,
			Args:        []string{"php"},
			SymfonyArgs: []string{"/test/symfony", "php"},
		},
	}

	ctx := t.Context()

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.Name, func(t *testing.T) {
			t.Run("Default", func(t *testing.T) {
				pathToSymfonyCLI = func() string { return "" }

				cmd := tc.Func(ctx, "some", "arguments")
				assert.Equal(t, append(tc.Args, "some", "arguments"), cmd.Args)
			})

			t.Run("Symfony", func(t *testing.T) {
				pathToSymfonyCLI = func() string { return "/test/symfony" }

				cmd := tc.Func(ctx, "some", "arguments")
				assert.Equal(t, append(tc.SymfonyArgs, "some", "arguments"), cmd.Args)
			})

			t.Run("Symfony disabled", func(t *testing.T) {
				t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

				pathToSymfonyCLI = func() string { return "/test/symfony" }

				cmd := tc.Func(ctx, "some", "arguments")
				assert.Equal(t, append(tc.Args, "some", "arguments"), cmd.Args)
			})
		})
	}
}
