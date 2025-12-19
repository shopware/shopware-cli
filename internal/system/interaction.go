package system

import "context"

type interactionKey struct{}

// WithInteraction returns a new context with the interaction state set.
func WithInteraction(ctx context.Context, interaction bool) context.Context {
	return context.WithValue(ctx, interactionKey{}, interaction)
}

// IsInteractionEnabled returns true if interaction is enabled in the context.
// It defaults to true if not set or if the stored value is not a bool.
func IsInteractionEnabled(ctx context.Context) bool {
	v := ctx.Value(interactionKey{})
	if v == nil {
		return true
	}

	if interaction, ok := v.(bool); ok {
		return interaction
	}

	return true
}
