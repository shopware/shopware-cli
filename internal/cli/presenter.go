package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Presenter is the single output channel commands use. Replacing the ~112
// scattered fmt.Print* calls with one presenter is the prerequisite (called out
// in architecture.md §5.1) for a uniform --output=json across every command.
//
// A command returns its result type and calls Result exactly once — it never
// prints directly. The human renderer prints to stdout; the JSON renderer
// serializes to stdout and reserves logs for stderr.
type Presenter interface {
	// Result renders a structured result. JSON mode marshals v; human mode hands
	// v to the HumanRenderFunc (defaulting to indented JSON).
	Result(v any)
	// Success prints a short human success line; JSON mode emits {status,message}.
	Success(msg string)
	// Error prints an error. Human mode writes to stderr; JSON mode emits a
	// structured {error: ...} envelope to stdout.
	Error(err error)
}

// HumanRenderFunc customizes how a result value prints in human mode. nil falls
// back to indented JSON, so every command is structured even before a tailored
// human view ships.
type HumanRenderFunc func(w io.Writer, v any) error

type presenter struct {
	w      io.Writer
	errW   io.Writer
	format OutputFormat
	human  HumanRenderFunc
}

// NewPresenter builds a Presenter for the given sink and format. errW is where
// Error writes in human mode (stderr). human may be nil.
func NewPresenter(w io.Writer, format OutputFormat, errW io.Writer, human HumanRenderFunc) Presenter {
	return &presenter{w: w, errW: errW, format: format, human: human}
}

// defaultHumanRender pretty-prints a result as JSON — the fallback when no
// HumanRenderFunc is set, keeping every command structured.
func defaultHumanRender(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *presenter) Result(v any) {
	switch p.format {
	case OutputJSON:
		_ = json.NewEncoder(p.w).Encode(v)
	default:
		r := p.human
		if r == nil {
			r = defaultHumanRender
		}
		_ = r(p.w, v)
	}
}

func (p *presenter) Success(msg string) {
	if p.format == OutputJSON {
		_ = json.NewEncoder(p.w).Encode(map[string]string{"status": "ok", "message": msg})
		return
	}
	_, _ = fmt.Fprintln(p.w, msg)
}

func (p *presenter) Error(err error) {
	if p.format == OutputJSON {
		_ = json.NewEncoder(p.w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_, _ = fmt.Fprintln(p.errW, "Error:", err)
}

// --- context plumbing ------------------------------------------------------

type (
	presenterKey struct{}
	stdoutKey    struct{}
	stderrKey    struct{}
)

// WithStd installs stdout/stderr sinks into context for the fallback presenter.
func WithStd(ctx context.Context, stdout, stderr io.Writer) context.Context {
	ctx = context.WithValue(ctx, stdoutKey{}, stdout)
	ctx = context.WithValue(ctx, stderrKey{}, stderr)
	return ctx
}

// WithPresenterCtx stores a Presenter in context.
func WithPresenterCtx(ctx context.Context, p Presenter) context.Context {
	return context.WithValue(ctx, presenterKey{}, p)
}

// FromContext returns the Presenter installed by the middleware chain. It falls
// back to a human presenter on the std sinks from WithStd so a command that
// predates the migration still gets human output rather than panicking.
func FromContext(ctx context.Context) Presenter {
	if p, ok := ctx.Value(presenterKey{}).(Presenter); ok {
		return p
	}
	out, _ := ctx.Value(stdoutKey{}).(io.Writer)
	errW, _ := ctx.Value(stderrKey{}).(io.Writer)
	return NewPresenter(out, OutputHuman, errW, nil)
}
