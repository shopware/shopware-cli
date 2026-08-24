package system

import "context"

type tuiKey struct{}

// WithTUI marks ctx as originating from a shopware-cli TUI. Docker compose
// exec keeps -T for commands launched with this context so they do not steal
// the TTY from the TUI (e.g. project dev).
func WithTUI(ctx context.Context) context.Context {
	return context.WithValue(ctx, tuiKey{}, true)
}

// IsTUI reports whether ctx was marked with WithTUI.
func IsTUI(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	v, ok := ctx.Value(tuiKey{}).(bool)
	return ok && v
}
