package logging

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestIsVerbose(t *testing.T) {
	t.Run("returns false when context value is not set", func(t *testing.T) {
		assert.False(t, IsVerbose(t.Context()))
	})

	t.Run("returns true when context value is set to true", func(t *testing.T) {
		ctx := WithVerbose(t.Context(), true)

		assert.True(t, IsVerbose(ctx))
	})

	t.Run("returns false when context value is set to false", func(t *testing.T) {
		ctx := WithVerbose(t.Context(), false)

		assert.False(t, IsVerbose(ctx))
	})

	t.Run("returns false when context value is invalid type", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), verboseKey, "invalid")

		assert.False(t, IsVerbose(ctx))
	})
}

func TestFromContextConcurrentFallback(t *testing.T) {
	const callers = 32
	loggers := make([]*zap.SugaredLogger, callers)
	start := make(chan struct{})
	var group sync.WaitGroup

	for i := range callers {
		group.Go(func() {
			<-start
			loggers[i] = FromContext(t.Context())
		})
	}

	close(start)
	group.Wait()

	require.NotNil(t, loggers[0])
	for _, logger := range loggers[1:] {
		assert.Same(t, loggers[0], logger)
	}
}
