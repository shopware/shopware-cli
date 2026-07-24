package prompt

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func threeChoices() *Overlay {
	return New(Options{
		ID:      "leave",
		Title:   "Leaving",
		Message: "Really?",
		Choices: []Choice{
			{ID: "stop", Label: "Stop & quit"},
			{ID: "quit", Label: "Quit, keep running"},
			{ID: "cancel", Label: "Cancel"},
		},
	})
}

func press(o *Overlay, key tea.Key) {
	_, _ = o.Update(tea.KeyPressMsg(key))
}

func TestPrompt_DefaultsToFirstChoice(t *testing.T) {
	o := threeChoices()
	assert.Equal(t, 0, o.selected)

	// Out-of-range default falls back to the first choice.
	o = New(Options{Choices: []Choice{{ID: "a", Label: "A"}}, Default: 5})
	assert.Equal(t, 0, o.selected)
}

func TestPrompt_ArrowsMoveAndClamp(t *testing.T) {
	o := threeChoices()

	press(o, tea.Key{Code: tea.KeyRight})
	assert.Equal(t, 1, o.selected)
	press(o, tea.Key{Code: tea.KeyRight})
	press(o, tea.Key{Code: tea.KeyRight})
	assert.Equal(t, 2, o.selected, "right must clamp at the last choice")

	press(o, tea.Key{Code: tea.KeyLeft})
	assert.Equal(t, 1, o.selected)
	press(o, tea.Key{Code: 'h', Text: "h"})
	press(o, tea.Key{Code: 'h', Text: "h"})
	assert.Equal(t, 0, o.selected, "left must clamp at the first choice")

	press(o, tea.Key{Code: 'l', Text: "l"})
	assert.Equal(t, 1, o.selected, "'l' advances the selection")
}

func TestPrompt_TabWraps(t *testing.T) {
	o := threeChoices()
	for _, want := range []int{1, 2, 0} {
		press(o, tea.Key{Code: tea.KeyTab})
		assert.Equal(t, want, o.selected)
	}
}

func TestPrompt_EnterEmitsSelectedChoice(t *testing.T) {
	o := threeChoices()
	press(o, tea.Key{Code: tea.KeyRight})

	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the prompt")
	require.NotNil(t, cmd)

	res, ok := cmd().(ResultMsg)
	require.True(t, ok, "expected ResultMsg, got %T", cmd())
	assert.Equal(t, "leave", res.ID)
	assert.Equal(t, "quit", res.Choice)
}

func TestPrompt_EscEmitsEmptyChoice(t *testing.T) {
	o := threeChoices()

	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.Nil(t, next, "esc should dismiss the prompt")
	require.NotNil(t, cmd)

	res, ok := cmd().(ResultMsg)
	require.True(t, ok)
	assert.Equal(t, "leave", res.ID)
	assert.Empty(t, res.Choice, "esc means dismissed without a choice")
}

func TestPrompt_DefaultsYesNo(t *testing.T) {
	o := New(Options{Title: "Sure?"})

	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next)
	res := cmd().(ResultMsg)
	assert.Equal(t, "confirm", res.ID, "default overlay ID")
	assert.Equal(t, "yes", res.Choice, "default choices are yes/no")
}

func TestPrompt_NonKeyMsgIsIgnored(t *testing.T) {
	o := threeChoices()
	next, cmd := o.Update(struct{}{})
	assert.Same(t, o, next)
	assert.Nil(t, cmd)
}

func TestPrompt_ViewRendersTitleMessageAndButtons(t *testing.T) {
	o := threeChoices()

	view := ansi.Strip(o.View(120, 40))
	assert.Contains(t, view, "Leaving")
	assert.Contains(t, view, "Really?")
	assert.Contains(t, view, "Stop & quit")
	assert.Contains(t, view, "Quit, keep running")
	assert.Contains(t, view, "Cancel")
}
