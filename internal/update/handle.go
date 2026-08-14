package update

import (
	"context"
	"sync"
)

type CheckResult struct {
	Release *ReleaseInfo
	Err     error
}

// CheckHandle allows the root command and a TUI to await the same result.
type CheckHandle struct {
	done   chan struct{}
	once   sync.Once
	result CheckResult
}

type contextKey struct{}

func NewCheckHandle() *CheckHandle {
	return &CheckHandle{done: make(chan struct{})}
}

func (h *CheckHandle) Complete(result CheckResult) {
	h.once.Do(func() {
		h.result = result
		close(h.done)
	})
}

func (h *CheckHandle) Wait(ctx context.Context) CheckResult {
	select {
	case <-h.done:
		return h.result
	case <-ctx.Done():
		return CheckResult{Err: ctx.Err()}
	}
}

func WithHandle(ctx context.Context, handle *CheckHandle) context.Context {
	return context.WithValue(ctx, contextKey{}, handle)
}

func HandleFromContext(ctx context.Context) *CheckHandle {
	handle, _ := ctx.Value(contextKey{}).(*CheckHandle)
	return handle
}
