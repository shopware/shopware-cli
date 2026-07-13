package tui

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func newTestModel(items []FilterSelectItem) *filterSelectModel {
	ti := textinput.New()
	ti.Focus()
	m := &filterSelectModel{items: items, filter: ti, pageSize: 10}
	m.applyFilter()
	return m
}

func TestFilterSelect_FilterMatchesLabelAndDetail(t *testing.T) {
	m := newTestModel([]FilterSelectItem{
		{Label: "Main Store", Detail: "https://main.test", Value: "1"},
		{Label: "EU Outlet", Detail: "https://eu.example.test", Value: "2"},
		{Label: "Demo", Detail: "https://demo.test", Value: "3"},
	})

	m.filter.SetValue("example")
	m.applyFilter()
	assert.Len(t, m.filtered, 1, "filter should match Detail (URL)")
	assert.Equal(t, 1, m.filtered[0])

	m.filter.SetValue("demo")
	m.applyFilter()
	assert.Len(t, m.filtered, 1, "filter should match Label")
	assert.Equal(t, 2, m.filtered[0])

	m.filter.SetValue("")
	m.applyFilter()
	assert.Len(t, m.filtered, 3, "empty filter keeps all items")
}

func TestFilterSelect_EnterPicksHighlighted(t *testing.T) {
	m := newTestModel([]FilterSelectItem{
		{Label: "A", Value: "a"},
		{Label: "B", Value: "b"},
		{Label: "C", Value: "c"},
	})

	// Move to second item, confirm.
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.NotNil(t, cmd, "enter should produce a Quit cmd")
	assert.False(t, m.cancelled)
	assert.Equal(t, 1, m.chosen)
}

func TestFilterSelect_EscCancels(t *testing.T) {
	m := newTestModel([]FilterSelectItem{{Label: "Only", Value: "only"}})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.NotNil(t, cmd)
	assert.True(t, m.cancelled)
}

func TestFilterSelect_WindowingClampsScroll(t *testing.T) {
	items := make([]FilterSelectItem, 25)
	for i := range items {
		items[i] = FilterSelectItem{Label: string(rune('a' + i)), Value: string(rune('a' + i))}
	}
	m := newTestModel(items)
	m.pageSize = 5

	for range 7 {
		_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	assert.Equal(t, 7, m.cursor)
	assert.Equal(t, 3, m.scroll, "scroll should follow cursor past the page")
}
