//go:build !windows

package project

import (
	"context"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This is the only thing standing between Ctrl+C and abandoned worker
// children: the signal must cancel the context instead of killing the process.
func TestCancelOnTerminationCancelsContext(t *testing.T) {
	// cancelOnTermination never unregisters its handler, so restore the default
	// disposition to keep the rest of the test binary interruptible.
	t.Cleanup(func() { signal.Reset(syscall.SIGTERM, syscall.SIGINT) })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cancelOnTermination(ctx, cancel)
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled after SIGINT")
	}
}
