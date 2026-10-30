package shop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduledTaskRunArgs(t *testing.T) {
	writeLock := func(t *testing.T, version string) string {
		t.Helper()
		dir := t.TempDir()
		lock := `{"packages":[{"name":"shopware/core","version":"` + version + `"}]}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
		return dir
	}

	t.Run("modern version uses no-wait", func(t *testing.T) {
		assert.Equal(t, []string{"scheduled-task:run", "--no-wait"}, ScheduledTaskRunArgs(writeLock(t, "6.7.3.1"), false))
	})

	t.Run("legacy version uses time limit", func(t *testing.T) {
		assert.Equal(t, []string{"scheduled-task:run", "--time-limit=1"}, ScheduledTaskRunArgs(writeLock(t, "6.4.20.2"), false))
	})

	t.Run("unknown version uses no-wait", func(t *testing.T) {
		assert.Equal(t, []string{"scheduled-task:run", "--no-wait", "-vvv"}, ScheduledTaskRunArgs(t.TempDir(), true))
	})
}

func TestRunScheduledTasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires the true and sleep binaries")
	}

	t.Run("invalid concurrency", func(t *testing.T) {
		require.Error(t, RunScheduledTasks(t.Context(), time.Millisecond, 0, time.Hour, 0, nil))
	})

	t.Run("runs immediately and on every tick", func(t *testing.T) {
		var runs atomic.Int32

		start := func(ctx context.Context) (*exec.Cmd, error) {
			runs.Add(1)
			return exec.CommandContext(ctx, "true"), nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
		defer cancel()

		require.NoError(t, RunScheduledTasks(ctx, 50*time.Millisecond, 0, time.Hour, 2, start))

		// One immediate run plus ~4 ticks.
		assert.GreaterOrEqual(t, runs.Load(), int32(4))
	})

	t.Run("jitter delays each run once", func(t *testing.T) {
		var runs atomic.Int32

		start := func(ctx context.Context) (*exec.Cmd, error) {
			runs.Add(1)
			return exec.CommandContext(ctx, "true"), nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		require.NoError(t, RunScheduledTasks(ctx, time.Hour, 50*time.Millisecond, time.Hour, 2, start))

		assert.Equal(t, int32(1), runs.Load())
	})

	t.Run("cancel during jitter skips the run", func(t *testing.T) {
		var runs atomic.Int32

		start := func(ctx context.Context) (*exec.Cmd, error) {
			runs.Add(1)
			return exec.CommandContext(ctx, "true"), nil
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		require.NoError(t, RunScheduledTasks(ctx, time.Hour, time.Hour, time.Hour, 2, start))

		assert.Equal(t, int32(0), runs.Load())
	})

	t.Run("caps concurrent runs", func(t *testing.T) {
		var mu sync.Mutex
		active, maxActive, runs := 0, 0, 0

		start := func(ctx context.Context) (*exec.Cmd, error) {
			mu.Lock()
			active++
			runs++
			maxActive = max(maxActive, active)
			mu.Unlock()

			cmd := exec.CommandContext(ctx, "sleep", "10")
			cmd.Cancel = func() error {
				mu.Lock()
				active--
				mu.Unlock()
				return cmd.Process.Kill()
			}

			return cmd, nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		defer cancel()

		require.NoError(t, RunScheduledTasks(ctx, 20*time.Millisecond, 0, time.Hour, 2, start))

		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 2, maxActive)
		assert.Equal(t, 2, runs)
	})

	t.Run("start errors free the slot", func(t *testing.T) {
		var attempts atomic.Int32

		start := func(_ context.Context) (*exec.Cmd, error) {
			attempts.Add(1)
			return nil, errors.New("boom")
		}

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		require.NoError(t, RunScheduledTasks(ctx, 20*time.Millisecond, 0, time.Hour, 1, start))

		assert.GreaterOrEqual(t, attempts.Load(), int32(3))
	})

	t.Run("stops runs that exceed the run timeout", func(t *testing.T) {
		var runs atomic.Int32
		var prev *exec.Cmd

		start := func(ctx context.Context) (*exec.Cmd, error) {
			// With a single slot, the previous run must be reaped before the next one starts.
			if prev != nil {
				assert.NotNil(t, prev.ProcessState)
			}

			runs.Add(1)
			prev = exec.CommandContext(ctx, "sleep", "10")

			return prev, nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		defer cancel()

		require.NoError(t, RunScheduledTasks(ctx, 20*time.Millisecond, 0, 50*time.Millisecond, 1, start))

		// Without the run timeout, the first run would hold the only slot.
		assert.GreaterOrEqual(t, runs.Load(), int32(3))
	})
}
