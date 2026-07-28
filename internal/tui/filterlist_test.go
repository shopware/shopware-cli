package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleFilterItems() []FilterItem {
	return []FilterItem{
		{Label: "Alpha", Detail: "first", Value: "a"},
		{Label: "Beta", Detail: "second", Value: "b"},
		{Label: "Gamma", Detail: "third", Value: "g"},
	}
}

func pressKey(l FilterList, code rune, text string) FilterList {
	next, _ := l.Update(tea.KeyPressMsg(tea.Key{Code: code, Text: text}))
	return next
}

func TestFilterListCursorMovement(t *testing.T) {
	l := NewFilterList(FilterListOptions{Items: sampleFilterItems()})

	item, index, ok := l.Selected()
	require.True(t, ok)
	assert.Equal(t, 0, index)
	assert.Equal(t, "Alpha", item.Label)

	l = pressKey(l, tea.KeyDown, "")
	l = pressKey(l, tea.KeyDown, "")
	_, index, _ = l.Selected()
	assert.Equal(t, 2, index)

	// Down past the end is a no-op.
	l = pressKey(l, tea.KeyDown, "")
	_, index, _ = l.Selected()
	assert.Equal(t, 2, index)

	l = pressKey(l, tea.KeyUp, "")
	l = pressKey(l, tea.KeyUp, "")
	l = pressKey(l, tea.KeyUp, "")
	_, index, _ = l.Selected()
	assert.Equal(t, 0, index, "up clamps at the first row")
}

func TestFilterListInitialIndex(t *testing.T) {
	l := NewFilterList(FilterListOptions{Items: sampleFilterItems(), InitialIndex: 2})
	item, index, ok := l.Selected()
	require.True(t, ok)
	assert.Equal(t, 2, index)
	assert.Equal(t, "Gamma", item.Label)

	// Out-of-range preselection falls back to the first row.
	l = NewFilterList(FilterListOptions{Items: sampleFilterItems(), InitialIndex: 99})
	_, index, _ = l.Selected()
	assert.Equal(t, 0, index)
}

func TestFilterListFiltering(t *testing.T) {
	l := NewFilterList(FilterListOptions{Items: sampleFilterItems(), InitialIndex: 2})
	for _, r := range "be" {
		l = pressKey(l, r, string(r))
	}

	assert.Equal(t, 1, l.Len(), `"be" matches only Beta`)
	item, index, ok := l.Selected()
	require.True(t, ok)
	assert.Equal(t, 1, index, "selection returns the original index")
	assert.Equal(t, "b", item.Value)

	// Details are searchable too.
	l = NewFilterList(FilterListOptions{Items: sampleFilterItems()})
	for _, r := range "third" {
		l = pressKey(l, r, string(r))
	}
	item, _, _ = l.Selected()
	assert.Equal(t, "Gamma", item.Label)

	// No match: Selected reports false.
	for _, r := range "zzz" {
		l = pressKey(l, r, string(r))
	}
	_, _, ok = l.Selected()
	assert.False(t, ok)
	assert.Equal(t, 0, l.Len())
}

func TestFilterListView(t *testing.T) {
	l := NewFilterList(FilterListOptions{Items: sampleFilterItems(), Header: "Name"})
	view := ansi.Strip(l.View(40))

	assert.Contains(t, view, "Name")
	assert.Contains(t, view, "Alpha")
	assert.Contains(t, view, "first")
	assert.Contains(t, view, "Gamma")
	assert.NotContains(t, view, "Showing", "no overflow line when everything fits")

	// Windowing kicks in past the page size.
	many := make([]FilterItem, 30)
	for i := range many {
		many[i] = FilterItem{Label: string(rune('A' + i))}
	}
	l = NewFilterList(FilterListOptions{Items: many, PageSize: 5})
	view = ansi.Strip(l.View(40))
	assert.Contains(t, view, "Showing 1–5 of 30")

	empty := NewFilterList(FilterListOptions{Items: nil})
	assert.Contains(t, ansi.Strip(empty.View(40)), "No matching items")
}
