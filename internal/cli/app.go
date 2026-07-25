package cli

import (
	"github.com/spf13/cobra"
)

// App is the assembled command tree plus its dependency wiring. It is the
// explicit-tree replacement for the 59 init()-based self-registrations measured
// in cmd/: commands are constructed, not globally mutated, so they are testable
// in isolation (architecture.md §2.5/§2.6).
type App struct {
	root  *cobra.Command
	deps  Deps
	chain Middleware
}

// NewApp builds an App over an existing root command. The middleware chain is
// composed once and installed as the root PersistentPreRunE, so every added
// command inherits logging, interaction, telemetry, presenter, and panic
// recovery — without any per-command remembering (the convention-vs-structure
// fix from architecture.md §7).
//
// stdout/stderr/human let the caller supply the real sinks and an optional app
// wide human renderer. Pass nil for human to use the structured default.
func NewApp(deps Deps, root *cobra.Command, stdout, stderr interface {
	Write([]byte) (int, error)
}, human HumanRenderFunc) *App {
	w, errW := sinkWriter(stdout), sinkWriter(stderr)

	chain := Chain(
		WithContextSetup(deps),
		WithPresenter(deps, w, errW, human),
		WithTelemetry(deps),
		// WithPanicRecover is innermost (closest to RunE) so a panic is converted
		// to an error before the surrounding WithTelemetry observes it; that way
		// Finish still runs and records ResultFailure.
		WithPanicRecover(),
	)

	// Install the chain as the root PreRun so every subcommand inherits it.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Wrap an identity run so PreRun itself does not need a RunE to fire the
		// chain. Commands supply their own RunE; the chain wraps that via
		// RunHandler at command construction time.
		return chain(func(c *cobra.Command, a []string) error { return nil })(cmd, args)
	}

	return &App{root: root, deps: deps, chain: chain}
}

// Root returns the assembled root command, ready for ExecuteContext.
func (a *App) Root() *cobra.Command { return a.root }

// Chain returns the composed middleware, exposed so a command constructor can
// wrap its own RunE with the same chain.
func (a *App) Chain() Middleware { return a.chain }

// AddCommand attaches a subcommand and wraps its RunE (and every descendant's
// RunE) with the app's middleware chain. Recursion is required because command
// groups like NewDemoRootCmd add their own subcommands before they reach us; if
// we only wrapped the top node, the leaf RunE would run outside the chain and
// bypass panic recovery, telemetry, and presenter setup. No init(), no global.
func (a *App) AddCommand(cmd *cobra.Command) {
	wrapRunE(cmd, a.chain)
	a.root.AddCommand(cmd)
}

// wrapRunE recursively wraps the RunE of cmd and all its descendants with the
// given chain. Commands without a RunE (pure parents) are skipped but still
// recursed into.
func wrapRunE(cmd *cobra.Command, chain Middleware) {
	if cmd.RunE != nil {
		orig := cmd.RunE
		cmd.RunE = chain(RunHandler(orig))
	}
	for _, sub := range cmd.Commands() {
		wrapRunE(sub, chain)
	}
}

// sinkWriter narrows a generic writer to io.Writer without an explicit cast at
// every call site. nil falls back to a discard writer.
func sinkWriter(w interface{ Write([]byte) (int, error) }) *narrowedWriter {
	if w == nil {
		return &narrowedWriter{}
	}
	return &narrowedWriter{w: w}
}

type narrowedWriter struct {
	w interface{ Write([]byte) (int, error) }
}

func (n *narrowedWriter) Write(p []byte) (int, error) {
	if n.w == nil {
		return len(p), nil
	}
	return n.w.Write(p)
}
