package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithInteraction(t *testing.T) {
	ctx := t.Context()

	ctxWithTrue := WithInteraction(ctx, true)
	assert.NotNil(t, ctxWithTrue)

	ctxWithFalse := WithInteraction(ctx, false)
	assert.NotNil(t, ctxWithFalse)
}

func TestIsInteractionEnabled(t *testing.T) {
	t.Run("returns true when context value is not set", func(t *testing.T) {
		result := IsInteractionEnabled(t.Context())
		assert.True(t, result)
	})

	t.Run("returns true when context value is set to true", func(t *testing.T) {
		ctx := WithInteraction(t.Context(), true)
		result := IsInteractionEnabled(ctx)
		assert.True(t, result)
	})

	t.Run("returns false when context value is set to false", func(t *testing.T) {
		ctx := WithInteraction(t.Context(), false)
		result := IsInteractionEnabled(ctx)
		assert.False(t, result)
	})

	t.Run("returns true when context value is invalid type", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), interactionKey{}, "invalid")
		result := IsInteractionEnabled(ctx)
		assert.True(t, result)
	})
}
