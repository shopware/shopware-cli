package picker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testKey struct{}

func sampleItems() []Item {
	return []Item{
		{Label: "Alpha", Detail: "first", Value: "a"},
		{Label: "Beta", Detail: "second", Value: "b"},
		{Label: "Gamma", Detail: "third", Value: "g"},
	}
}

// Preselection, cursor movement, clamping, and filter mechanics are covered
// by the tui.FilterList tests; here only the overlay's result contract is
// pinned.

func TestPicker_EnterEmitsResultWithIndexAndKey(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "T", Items: sampleItems(), InitialIndex: 1})

	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the picker")
	require.NotNil(t, cmd)

	res, ok := cmd().(ResultMsg)
	require.True(t, ok, "expected ResultMsg, got %T", cmd())
	assert.False(t, res.Cancelled)
	assert.Equal(t, 1, res.Index)
	assert.Equal(t, "b", res.Value)
	_, keyOK := res.Key.(testKey)
	assert.True(t, keyOK)
}

func TestPicker_EnterOnEmptyFilteredCancels(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "T", Items: sampleItems()})
	for _, r := range "zzzz" {
		next, _ := o.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		o = next.(*Overlay)
	}
	assert.Equal(t, 0, o.Len())

	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next)
	res := cmd().(ResultMsg)
	assert.True(t, res.Cancelled)
}

func TestPicker_EscEmitsCancelled(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "T", Items: sampleItems()})
	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.Nil(t, next, "esc should dismiss the picker")
	require.NotNil(t, cmd)

	res, ok := cmd().(ResultMsg)
	require.True(t, ok)
	assert.True(t, res.Cancelled)
	_, keyOK := res.Key.(testKey)
	assert.True(t, keyOK)
}

func TestPicker_TypingFiltersAndSelectsMatch(t *testing.T) {
	o := New(Options{Key: testKey{}, Title: "T", Items: sampleItems(), InitialIndex: 2})
	for _, r := range "be" {
		next, _ := o.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		o = next.(*Overlay)
	}

	// "be" matches "Beta"; enter reports its original index and value.
	next, cmd := o.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next)
	res := cmd().(ResultMsg)
	assert.False(t, res.Cancelled)
	assert.Equal(t, 1, res.Index)
	assert.Equal(t, "b", res.Value)
}

func TestPicker_ViewRendersTitleHelpAndRows(t *testing.T) {
	o := New(Options{Title: "Pick one", Help: "Choose carefully", Items: sampleItems(), Header: "Name"})

	view := ansi.Strip(o.View(120, 40))
	assert.Contains(t, view, "Pick one")
	assert.Contains(t, view, "Choose carefully")
	assert.Contains(t, view, "Name")
	assert.Contains(t, view, "Alpha")
}
