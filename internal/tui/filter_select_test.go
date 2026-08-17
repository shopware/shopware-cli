package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func newTestFilterSelect(items []FilterSelectItem) *filterSelectModel {
	filterItems := make([]FilterItem, len(items))
	for i, item := range items {
		filterItems[i] = FilterItem(item)
	}
	m := &filterSelectModel{list: NewFilterList(FilterListOptions{Items: filterItems})}
	m.Init()
	return m
}

func typeQuery(m *filterSelectModel, query string) {
	for _, r := range query {
		_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func TestFilterSelect_FilterMatchesLabelAndDetail(t *testing.T) {
	m := newTestFilterSelect([]FilterSelectItem{
		{Label: "Main Store", Detail: "https://main.test", Value: "1"},
		{Label: "EU Outlet", Detail: "https://eu.example.test", Value: "2"},
		{Label: "Demo", Detail: "https://demo.test", Value: "3"},
	})

	typeQuery(m, "example")
	assert.Equal(t, 1, m.list.Len(), "filter should match Detail (URL)")
	_, index, ok := m.list.Selected()
	assert.True(t, ok)
	assert.Equal(t, 1, index)
}

func TestFilterSelect_EnterPicksHighlighted(t *testing.T) {
	m := newTestFilterSelect([]FilterSelectItem{
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
	m := newTestFilterSelect([]FilterSelectItem{{Label: "Only", Value: "only"}})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.NotNil(t, cmd)
	assert.True(t, m.cancelled)
}

func TestFilterSelect_EnterWithoutMatchCancels(t *testing.T) {
	m := newTestFilterSelect([]FilterSelectItem{{Label: "Only", Value: "only"}})

	typeQuery(m, "zzz")
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.NotNil(t, cmd)
	assert.True(t, m.cancelled)
}
