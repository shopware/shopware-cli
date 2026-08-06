package update

import (
	"context"
	"sync"
)

type CheckResult struct {
	Release *ReleaseInfo
	Err     error
}

type CheckHandle struct {
	done     chan struct{}
	once     sync.Once
	mu       sync.RWMutex
	result   CheckResult
	rendered bool
}

type contextKey struct{}

func NewCheckHandle() *CheckHandle { return &CheckHandle{done: make(chan struct{})} }

func (h *CheckHandle) Complete(result CheckResult) {
	h.once.Do(func() {
		h.mu.Lock()
		h.result = result
		h.mu.Unlock()
		close(h.done)
	})
}

func (h *CheckHandle) Wait(ctx context.Context) CheckResult {
	select {
	case <-h.done:
		h.mu.RLock()
		defer h.mu.RUnlock()
		return h.result
	case <-ctx.Done():
		return CheckResult{Err: ctx.Err()}
	}
}

func (h *CheckHandle) MarkRendered() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rendered {
		return false
	}
	h.rendered = true
	return true
}

func (h *CheckHandle) Rendered() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rendered
}

func WithHandle(ctx context.Context, h *CheckHandle) context.Context {
	return context.WithValue(ctx, contextKey{}, h)
}

func HandleFromContext(ctx context.Context) *CheckHandle {
	h, _ := ctx.Value(contextKey{}).(*CheckHandle)
	return h
}
