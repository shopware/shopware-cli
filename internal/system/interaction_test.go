package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithInteraction(t *testing.T) {
	ctx := context.Background()

	t.Run("sets interaction to true", func(t *testing.T) {
		newCtx := WithInteraction(ctx, true)
		value := newCtx.Value(interactionKey{})
		assert.NotNil(t, value)
		assert.Equal(t, true, value)
	})

	t.Run("sets interaction to false", func(t *testing.T) {
		newCtx := WithInteraction(ctx, false)
		value := newCtx.Value(interactionKey{})
		assert.NotNil(t, value)
		assert.Equal(t, false, value)
	})
}

func TestIsInteractionEnabled(t *testing.T) {
	t.Run("returns true when context value is not set", func(t *testing.T) {
		ctx := context.Background()
		assert.True(t, IsInteractionEnabled(ctx))
	})

	t.Run("returns true when interaction is set to true", func(t *testing.T) {
		ctx := WithInteraction(context.Background(), true)
		assert.True(t, IsInteractionEnabled(ctx))
	})

	t.Run("returns false when interaction is set to false", func(t *testing.T) {
		ctx := WithInteraction(context.Background(), false)
		assert.False(t, IsInteractionEnabled(ctx))
	})

	t.Run("returns true when context contains invalid type", func(t *testing.T) {
		// Create a context with an invalid type value
		ctx := context.WithValue(context.Background(), interactionKey{}, "invalid-string")
		assert.True(t, IsInteractionEnabled(ctx))
	})

	t.Run("returns true when context contains nil value explicitly", func(t *testing.T) {
		// This is a special case where we explicitly set nil (though practically unlikely)
		type testKey struct{}
		ctx := context.WithValue(context.Background(), testKey{}, nil)
		// Using our function on a context that doesn't have our key set
		assert.True(t, IsInteractionEnabled(ctx))
	})

	t.Run("interaction state persists through context chain", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithInteraction(ctx, false)
		
		// Add additional context values
		type otherKey struct{}
		ctx = context.WithValue(ctx, otherKey{}, "some-value")
		
		// Interaction state should still be preserved
		assert.False(t, IsInteractionEnabled(ctx))
	})

	t.Run("can toggle interaction state in context chain", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithInteraction(ctx, false)
		assert.False(t, IsInteractionEnabled(ctx))
		
		// Toggle to true
		ctx = WithInteraction(ctx, true)
		assert.True(t, IsInteractionEnabled(ctx))
		
		// Toggle back to false
		ctx = WithInteraction(ctx, false)
		assert.False(t, IsInteractionEnabled(ctx))
	})
}
