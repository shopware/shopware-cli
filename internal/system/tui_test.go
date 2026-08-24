package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithTUI(t *testing.T) {
	parent := t.Context()

	assert.False(t, IsTUI(parent))
	assert.False(t, IsTUI(context.Background()))
	assert.True(t, IsTUI(TUIContext()))

	ctx := WithTUI(parent)
	assert.True(t, IsTUI(ctx))

	derived, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	assert.True(t, IsTUI(derived), "WithCancel must keep the TUI mark")
}
