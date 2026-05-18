package devtui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

type testPickerKey struct{}

func sampleListItems() []listPickerItem {
	return []listPickerItem{
		{Label: "Alpha", Detail: "first", Value: "a"},
		{Label: "Beta", Detail: "second", Value: "b"},
		{Label: "Gamma", Detail: "third", Value: "g"},
	}
}

func TestNewListPicker_PreselectsInitialIndex(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "Title", "Help", sampleListItems(), 2)
	assert.Equal(t, 2, lp.cursor)
	assert.Equal(t, []int{0, 1, 2}, lp.filtered)
	assert.Equal(t, "Title", lp.title)
	assert.Equal(t, "Help", lp.help)
	assert.Len(t, lp.items, 3)
}

func TestNewListPicker_InitialIndexOutOfRangeFallsBackToZero(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 99)
	assert.Equal(t, 0, lp.cursor)
}

func TestListPicker_DownArrowMovesCursorWithinBounds(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 0)

	next, _ := lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	lp = next.(*listPicker)
	assert.Equal(t, 1, lp.cursor)

	next, _ = lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	lp = next.(*listPicker)
	assert.Equal(t, 2, lp.cursor)

	// Down past the end is a no-op
	next, _ = lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	lp = next.(*listPicker)
	assert.Equal(t, 2, lp.cursor)
}

func TestListPicker_UpArrowClampsAtZero(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 1)
	assert.Equal(t, 1, lp.cursor)

	next, _ := lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	lp = next.(*listPicker)
	assert.Equal(t, 0, lp.cursor)

	next, _ = lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	lp = next.(*listPicker)
	assert.Equal(t, 0, lp.cursor)
}

func TestListPicker_EnterEmitsResultWithIndexAndKey(t *testing.T) {
	key := testPickerKey{}
	lp := newListPicker(key, "T", "", sampleListItems(), 1)

	next, cmd := lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the picker")
	assert.NotNil(t, cmd)

	pr, ok := cmd().(pickerResultMsg)
	assert.True(t, ok, "expected pickerResultMsg, got %T", cmd())
	assert.False(t, pr.Cancelled)
	assert.Equal(t, 1, pr.Index)
	assert.Equal(t, "b", pr.Value)
	_, keyOK := pr.Key.(testPickerKey)
	assert.True(t, keyOK)
}

func TestListPicker_EnterOnEmptyFilteredCancels(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 0)
	for _, r := range "zzzz" {
		next, _ := lp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		lp = next.(*listPicker)
	}
	assert.Empty(t, lp.filtered)

	next, cmd := lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next)
	pr := cmd().(pickerResultMsg)
	assert.True(t, pr.Cancelled)
}

func TestListPicker_EscEmitsCancelled(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 0)
	next, cmd := lp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Nil(t, next, "esc should dismiss the picker")
	assert.NotNil(t, cmd)

	pr, ok := cmd().(pickerResultMsg)
	assert.True(t, ok)
	assert.True(t, pr.Cancelled)
	_, keyOK := pr.Key.(testPickerKey)
	assert.True(t, keyOK)
}

func TestListPicker_TypingFiltersAndKeepsCursorInRange(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "T", "", sampleListItems(), 2)
	for _, r := range "be" {
		next, _ := lp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		lp = next.(*listPicker)
	}
	// "be" matches "Beta"
	assert.Equal(t, []int{1}, lp.filtered)
	assert.Equal(t, 0, lp.cursor, "cursor must clamp into filtered range")
}

func TestListPicker_ViewRendersWithoutPanic(t *testing.T) {
	lp := newListPicker(testPickerKey{}, "Pick one", "Choose carefully", sampleListItems(), 0)
	assert.NotPanics(t, func() {
		view := lp.View(120, 40)
		assert.Contains(t, view, "Pick one")
		assert.Contains(t, view, "Alpha")
	})
}

func TestNewTextPicker_StoresInitialValue(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Enter name", "Help text", "preset", false)
	assert.Equal(t, "preset", tp.input.Value())
	assert.Equal(t, "Enter name", tp.title)
	assert.Equal(t, "Help text", tp.help)
	assert.False(t, tp.secret)
	assert.True(t, tp.input.Focused())
}

func TestTextPicker_TypingUpdatesValue(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Name", "", "", false)
	for _, r := range "abc" {
		next, _ := tp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		tp = next.(*textPicker)
	}
	assert.Equal(t, "abc", tp.input.Value())
}

func TestTextPicker_EnterEmitsValue(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Name", "", "hello", false)
	next, cmd := tp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the text picker")
	assert.NotNil(t, cmd)

	pr, ok := cmd().(pickerResultMsg)
	assert.True(t, ok)
	assert.False(t, pr.Cancelled)
	assert.Equal(t, "hello", pr.Value)
	_, keyOK := pr.Key.(testPickerKey)
	assert.True(t, keyOK)
}

func TestTextPicker_EscEmitsCancelled(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Name", "", "hello", false)
	next, cmd := tp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Nil(t, next)
	pr := cmd().(pickerResultMsg)
	assert.True(t, pr.Cancelled)
}

func TestTextPicker_SecretFlagIsStored(t *testing.T) {
	// NOTE: At time of writing the textPicker.View() does not actually use the
	// secret flag to mask the underlying textinput. This test pins the
	// constructor's contract (the flag is stored) so a future fix that turns
	// on EchoMode=password can be observed without changing this test.
	tp := newTextPicker(testPickerKey{}, "Password", "", "supersecret", true)
	assert.True(t, tp.secret)
	assert.NotPanics(t, func() { tp.View(120, 40) })
}

func TestTextPicker_NonSecretShowsValueInView(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Name", "", "visibleval", false)
	view := tp.View(120, 40)
	assert.True(t, strings.Contains(view, "visibleval"), "non-secret value should be visible in view")
}

func TestTextPicker_NonKeyMsgIsIgnored(t *testing.T) {
	tp := newTextPicker(testPickerKey{}, "Name", "", "x", false)
	next, cmd := tp.Update(struct{}{})
	assert.Same(t, tp, next)
	assert.Nil(t, cmd)
}
