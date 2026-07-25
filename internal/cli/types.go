// Package cli contains the recommended application scaffolding for shopware-cli:
// a lazy dependency container, a unified presenter, a middleware chain, and an
// explicit command-tree builder. It addresses the scalability blockers called
// out in architecture.md §2.5, §2.6, §5.1 and §7 without pulling in a framework.
//
// The pieces are deliberately tiny and framework-agnostic so they can be adopted
// incrementally: each file is usable on its own and independently testable.
//
// The companion test (app_test.go) wires a tiny `demo login` command through the
// full stack to prove the four recommended patterns work end-to-end.
package cli

import (
	"context"
	"sync"

	"github.com/spf13/cobra"
)

// CobraFunc is the shape of a cobra run function. Middleware wrap it.
type CobraFunc = func(cmd *cobra.Command, args []string) error

// --- Deps ----------------------------------------------------------------—

// Deps bundles everything the command layer needs at construction time. It is
// the generalized form of account.ServiceContainer (cmd/account) lifted to the
// whole application. Replace production Services/Logger/Telemetry with stubs in
// tests:
//
//	app := cli.NewApp(cli.Deps{Services: fake}, rootCmd)
type Deps struct {
	Services  Services
	Logger    Logger
	Telemetry Telemetry
	Options   Options
}

// Options are the global runtime flags resolved once in the root middleware.
type Options struct {
	// Output controls which presenter the chain installs into context.
	Output OutputFormat
	// Verbose enables debug logging (logging.WithVerbose).
	Verbose bool
	// Interactive mirrors system.IsInteractionEnabled, but resolved once and
	// threaded structurally so no command re-derives it (architecture.md §7).
	Interactive bool
}

// OutputFormat enumerates the --output values. Human is the default.
type OutputFormat string

const (
	OutputHuman OutputFormat = "human"
	OutputJSON  OutputFormat = "json"
)

// --- Logger / Telemetry ---------------------------------------------------

// Logger is the minimal logging surface the middleware needs. The production
// implementation is *zap.SugaredLogger from the logging package, which already
// satisfies this interface.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// nopLogger is the default when Deps.Logger is nil (e.g. tests).
type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// Telemetry measures a single command invocation. Finish is called exactly once
// by WithTelemetry; the production impl forwards to tracking.Track. Tests
// record calls for assertion.
type Telemetry interface {
	Finish(ctx context.Context, cmd CommandInfo, result ResultKind, durationMS int64)
}

// nopTelemetry discards events when Deps.Telemetry is nil.
type nopTelemetry struct{}

func (nopTelemetry) Finish(context.Context, CommandInfo, ResultKind, int64) {}

// CommandInfo is the slice of data the middleware extracts from each cobra run.
type CommandInfo struct {
	Name  string // e.g. "account.login"
	IsTUI bool
}

// ResultKind classifies how a run ended. Strings are telemetry-stable values,
// matching tracking.ResultSuccess/Failure/Cancelled.
type ResultKind string

const (
	ResultSuccess   ResultKind = "success"
	ResultFailure   ResultKind = "failure"
	ResultCancelled ResultKind = "cancelled"
)

// --- Services + Container (lazy getters) ---------------------------------//

// Services is the seam between the command layer and everything else. Each
// method is a lazy getter: the first call constructs the value, subsequent
// calls return the cached one. Commands get exactly what they ask for — a
// `login` command never pays the cost of building a shop client.
//
// To extend: add a method and a backing memoized getter on Container.
type Services interface {
	AccountClient(ctx context.Context) (AccountClient, error)
}

// AccountClient is the minimal abstraction consumed by the demo login command.
// In the real CLI this would be *account_api.Client.
type AccountClient interface {
	Login(ctx context.Context) (LoginResult, error)
}

// LoginResult is the structured result of a login. Every command returns a
// concrete result type rather than printing directly — this is the contract
// that makes --output=json uniform (presenter.Presenter).
type LoginResult struct {
	User string `json:"user"`
}

// container implements Services with lazy construction. The zero value is
// intentionally invalid; build with NewContainer.
type container struct {
	account func() (AccountClient, error)
}

// AccountClient satisfies Services.
func (c *container) AccountClient(context.Context) (AccountClient, error) {
	if c.account == nil {
		return nil, ErrNotConfigured("AccountClient")
	}
	return c.account()
}

// NewContainer builds a Container from factory closures. Only the factories a
// caller passes are ever invoked; unset services return ErrNotConfigured so a
// command that asks for an unset dependency fails loudly instead of nil-derefing.
func NewContainer(factories ContainerFactories) Services {
	c := &container{}
	if factories.AccountClient != nil {
		c.account = newMemo(factories.AccountClient)
	}
	return c
}

// ContainerFactories are inputs to NewContainer. A nil factory leaves that
// service unset.
type ContainerFactories struct {
	AccountClient func() (AccountClient, error)
	// add: ShopClient func(path string) (*shop.Client, error), ...
}

// memo wraps a once-style lazy getter. Subsequent calls reuse the cache.
func newMemo[T any](build func() (T, error)) func() (T, error) {
	var (
		once sync.Once
		val  T
		err  error
	)
	return func() (T, error) {
		once.Do(func() { val, err = build() })
		return val, err
	}
}

// ErrNotConfigured is returned when a command asks for an unwired service.
type ErrNotConfigured string

func (e ErrNotConfigured) Error() string { return "cli: service not configured: " + string(e) }
