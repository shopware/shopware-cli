package bootstrap

import (
	"cli/internal/config"
	"cli/internal/runtimectx"
	"context"

	"github.com/spf13/cobra"
)

// LoadConfigStep returns a bootstrap Step that loads the application
// configuration using config.Load and stores the result in the provided Context.
//
// It reads values from the given command's FlagSet (together with defaults,
// config file, and environment variables inside config.Load), merges them into
// a Config struct, and validates the result. The Config is then attached to the
// Context with runtimectx.WithConfig so that subsequent steps and subcommands
// can retrieve it via runtimectx.ConfigFrom.
//
// On failure, the returned step propagates the current Context unchanged along
// with the validation or loading error.
func LoadConfigStep() func(context.Context, *cobra.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cobra.Command) (context.Context, error) {
		cfg, err := config.Load(cmd.Flags())
		if err != nil {
			return ctx, err
		}
		return runtimectx.WithConfig(ctx, cfg), nil
	}
}
