package textprompt

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testKey struct{}

func TestTextPrompt_StoresInitialValue(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "Enter name", Help: "Help text", Value: "preset"})
	assert.Equal(t, "preset", o.Value())
}

func TestTextPrompt_TypingUpdatesValue(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "Name"})
	for _, r := range "abc" {
		next, _ := o.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		o = next.(*Overlay)
	}
	assert.Equal(t, "abc", o.Value())
}

func TestTextPrompt_EnterEmitsValue(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "Name", Value: "hello"})
	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the prompt")
	require.NotNil(t, cmd)

	res, ok := cmd().(ResultMsg)
	require.True(t, ok)
	assert.False(t, res.Cancelled)
	assert.Equal(t, "hello", res.Value)
	_, keyOK := res.Key.(testKey)
	assert.True(t, keyOK)
}

func TestTextPrompt_EscEmitsCancelled(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "Name", Value: "hello"})
	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.Nil(t, next)
	res := cmd().(ResultMsg)
	assert.True(t, res.Cancelled)
}

func TestTextPrompt_SecretMasksValueInView(t *testing.T) {
	o := New(Options{Title: "Password", Value: "supersecret", Secret: true})

	view := ansi.Strip(o.View(120, 40))
	assert.NotContains(t, view, "supersecret", "secret values must be masked")

	// The masked value still round-trips on confirm.
	_, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, "supersecret", cmd().(ResultMsg).Value)
}

func TestTextPrompt_NonSecretShowsValueInView(t *testing.T) {
	o := New(Options{Title: "Name", Value: "visibleval"})
	view := ansi.Strip(o.View(120, 40))
	assert.Contains(t, view, "visibleval")
	assert.Contains(t, view, "Name")
}

func TestTextPrompt_NonKeyMsgIsIgnored(t *testing.T) {
	o := New(Options{Title: "Name", Value: "x"})
	next, cmd := o.Update(struct{}{})
	assert.Same(t, o, next)
	assert.Nil(t, cmd)
}
