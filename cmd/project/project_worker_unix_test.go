//go:build !windows

package project

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gracefulStop is wired as exec.Cmd.Cancel for every messenger consumer;
// returning os.ErrProcessDone is load-bearing since exec treats it as a clean
// stop. Like in production, Cancel runs concurrently with Wait, which reaps
// the child so the poll loop can observe the exit.
func TestGracefulStopSigtermThenProcessDone(t *testing.T) {
	t.Run("sigterm stops within the grace period", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sleep", "30")
		require.NoError(t, cmd.Start())
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()

		start := time.Now()
		err := gracefulStop(cmd, 5)
		require.ErrorIs(t, err, os.ErrProcessDone)
		assert.Less(t, time.Since(start), 3*time.Second)

		<-waitDone
	})

	t.Run("limit zero kills immediately", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), "sleep", "30")
		require.NoError(t, cmd.Start())
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()

		require.NoError(t, gracefulStop(cmd, 0))
		<-waitDone
	})
}
