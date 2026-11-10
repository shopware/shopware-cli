//go:build unix

package verifier

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCancellationStopsWorkerProcesses(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := CommandContext(ctx, "sh", "-c", "sleep 30 & printf ready; wait")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	ready := make([]byte, len("ready"))
	_, err = io.ReadFull(stdout, ready)
	require.NoError(t, err)
	cancel()
	// The worker inherited stdout. EOF proves it released that pipe too.
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(stdout)
		done <- err
	}()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("worker kept stdout open after cancellation")
	}
	assert.Error(t, cmd.Wait())
}
