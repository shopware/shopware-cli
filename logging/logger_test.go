package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
