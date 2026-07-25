package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

// Middleware wraps a cobra run function. The chain is built once in NewApp and
// installed as the root PersistentPreRunE so every command inherits it instead
// of re-implementing setup (architecture.md §2.6).
type Middleware func(next CobraFunc) CobraFunc

// Chain composes middlewares right-to-left: Chain(a, b)(fn) == a(b(fn)). The
// first listed middleware runs outermost, so it can set up context (logger,
// presenter, interaction) before inner ones read it, and clean up last.
func Chain(mws ...Middleware) Middleware {
	return func(next CobraFunc) CobraFunc {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// RunHandler binds a CobraFunc to cobra's RunE signature.
func RunHandler(fn CobraFunc) func(cmd *cobra.Command, args []string) error {
	return fn
}

// --- built-in middleware --------------------------------------------------

// WithContextSetup installs logger and interaction mode into the command
// context. This is the root setup currently inlined in cmd/root.go run(); here
// it is testable and composable.
func WithContextSetup(deps Deps) Middleware {
	return func(next CobraFunc) CobraFunc {
		return func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ctx = logging.WithVerbose(ctx, deps.Options.Verbose)
			ctx = logging.WithLogger(ctx, logging.NewLogger(deps.Options.Verbose))
			ctx = system.WithInteraction(ctx, deps.Options.Interactive)
			cmd.SetContext(ctx)
			return next(cmd, args)
		}
	}
}

// WithPresenter installs stdout/stderr sinks and a Presenter (chosen by
// deps.Options.Output) into context. human optionally customizes human output.
func WithPresenter(deps Deps, w io.Writer, errW io.Writer, human HumanRenderFunc) Middleware {
	return func(next CobraFunc) CobraFunc {
		return func(cmd *cobra.Command, args []string) error {
			ctx := WithStd(cmd.Context(), w, errW)
			ctx = WithPresenterCtx(ctx, NewPresenter(w, deps.Options.Output, errW, human))
			cmd.SetContext(ctx)
			return next(cmd, args)
		}
	}
}

// WithTelemetry records start time and calls deps.Telemetry.Finish exactly once
// with the resolved CommandInfo, result kind, and duration. Replaces the inline
// rollup at the bottom of cmd/root.go run().
func WithTelemetry(deps Deps) Middleware {
	t := deps.Telemetry
	if t == nil {
		t = nopTelemetry{}
	}
	return func(next CobraFunc) CobraFunc {
		return func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			info := CommandInfo{Name: commandName(cmd)}
			err := next(cmd, args)
			result := ResultSuccess
			switch {
			case err == nil:
				result = ResultSuccess
			case errors.Is(err, context.Canceled):
				result = ResultCancelled
			default:
				result = ResultFailure
			}
			t.Finish(cmd.Context(), info, result, time.Since(start).Milliseconds())
			return err
		}
	}
}

// WithPanicRecover converts a panic into a returned error so telemetry and
// presenter still run and a panic never escapes to os.Exit.
func WithPanicRecover() Middleware {
	return func(next CobraFunc) CobraFunc {
		return func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return next(cmd, args)
		}
	}
}

// commandName mirrors cmd/root.go normalization (CommandPath → spaced → dotted →
// snake) so telemetry names stay stable across the migration.
func commandName(cmd *cobra.Command) string {
	name := strings.TrimPrefix(cmd.CommandPath(), "shopware-cli ")
	name = strings.ReplaceAll(name, " ", ".")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}
