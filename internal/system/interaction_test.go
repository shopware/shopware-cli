package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithInteraction(t *testing.T) {
	ctx := context.Background()

	ctxWithTrue := WithInteraction(ctx, true)
	assert.NotNil(t, ctxWithTrue)

	ctxWithFalse := WithInteraction(ctx, false)
	assert.NotNil(t, ctxWithFalse)
}

func TestIsInteractionEnabled(t *testing.T) {
	t.Run("returns true when context value is not set", func(t *testing.T) {
		result := IsInteractionEnabled(context.Background())
		assert.True(t, result)
	})

	t.Run("returns true when context value is set to true", func(t *testing.T) {
		ctx := WithInteraction(context.Background(), true)
		result := IsInteractionEnabled(ctx)
		assert.True(t, result)
	})

	t.Run("returns false when context value is set to false", func(t *testing.T) {
		ctx := WithInteraction(context.Background(), false)
		result := IsInteractionEnabled(ctx)
		assert.False(t, result)
	})

	t.Run("returns true when context value is invalid type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), interactionKey{}, "invalid")
		result := IsInteractionEnabled(ctx)
		assert.True(t, result)
	})
}
