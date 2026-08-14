package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpdateCheckCmdReturnsUpdateMessage(t *testing.T) {
	called := false
	cmd := NewUpdateCheckCmd(func(ctx context.Context) bool {
		called = true
		assert.NotNil(t, ctx)
		return true
	})

	msg := cmd()

	assert.True(t, called)
	assert.IsType(t, UpdateAvailableMsg{}, msg)
}

func TestNewUpdateCheckCmdReturnsNoMessageWhenUnavailable(t *testing.T) {
	cmd := NewUpdateCheckCmd(func(context.Context) bool { return false })

	var msg tea.Msg = cmd()

	assert.Nil(t, msg)
}

func TestNewUpdateCheckCmdHandlesNilWaiter(t *testing.T) {
	assert.Nil(t, NewUpdateCheckCmd(nil))
}

func TestHeaderUpdatesWhenUpdateIsAvailable(t *testing.T) {
	header := NewHeader()
	updated, cmd := header.Update(UpdateAvailableMsg{})

	require.Nil(t, cmd)
	assert.Contains(t, updated.View(200), "Update available")
}
