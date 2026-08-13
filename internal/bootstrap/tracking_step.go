package bootstrap

import (
	"cli/internal/logger"
	"cli/internal/runtimectx"
	"context"

	"github.com/spf13/cobra"
)

// LoadLoggerStep returns a bootstrap Step that initializes a logger
// based on the configuration already present in the Context.
//
// The step calls logging.Load with the current Context to build
// a configured *slog.Logger (e.g. JSON vs. text output, log level).
// The resulting logger is then attached to the Context using
// runtimectx.WithLogger, making it available to subsequent steps and
// subcommands via runtimectx.LoggerFrom.
//
// On failure, the step propagates the current Context unchanged together
// with the logger initialization error.
func LoadLoggerStep() func(context.Context, *cobra.Command) (context.Context, error) {
	return func(ctx context.Context, cmd *cobra.Command) (context.Context, error) {
		l, err := logger.Load(ctx)
		if err != nil {
			return ctx, err
		}
		return runtimectx.WithLogger(ctx, *l), nil
	}
}
