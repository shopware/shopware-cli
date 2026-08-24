package system

import "context"

type tuiKey struct{}

// WithTUI marks ctx as originating from a shopware-cli TUI. Docker compose
// exec keeps -T for commands launched with this context so they do not steal
// the TTY from the TUI (e.g. project dev).
func WithTUI(ctx context.Context) context.Context {
	return context.WithValue(ctx, tuiKey{}, true)
}

// TUIContext returns a background context marked with WithTUI.
func TUIContext() context.Context {
	return WithTUI(context.Background())
}

// IsTUI reports whether ctx was marked with WithTUI.
func IsTUI(ctx context.Context) bool {
	v, ok := ctx.Value(tuiKey{}).(bool)
	return ok && v
}
