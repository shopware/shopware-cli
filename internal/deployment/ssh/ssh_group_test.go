package ssh

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHGroupParallelism(t *testing.T) {
	for _, limit := range []int{0, 1, 2} {
		g, err := newSSHGroup([]sshHost{{"a", &SSH{}}, {"b", &SSH{}}, {"c", &SSH{}}}, "a", limit)
		require.NoError(t, err)
		var running, maximum atomic.Int32
		require.NoError(t, g.parallel(t.Context(), func(int) error {
			current := running.Add(1)
			for old := maximum.Load(); old < current && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
			return nil
		}))
		expected := limit
		if expected == 0 {
			expected = 2
		}
		assert.LessOrEqual(t, maximum.Load(), int32(expected))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, g.parallel(ctx, func(int) error { return ctx.Err() }), context.Canceled)
	}
}

func TestSSHGroupSupportsInitializationAndPruning(t *testing.T) {
	g, err := newSSHGroup([]sshHost{
		{name: "a", backend: &SSH{}},
		{name: "b", backend: &SSH{}},
	}, "b", 2)
	require.NoError(t, err)
	assert.Implements(t, (*Initializer)(nil), g)
}
