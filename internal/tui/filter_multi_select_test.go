package tui

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func newTestMultiModel(items []FilterMultiSelectItem) *filterMultiSelectModel {
	ti := textinput.New()
	ti.Focus()
	m := &filterMultiSelectModel{items: items, filter: ti, pageSize: 10, selected: map[int]bool{}}
	m.applyFilter()
	return m
}

func TestFilterMultiSelect_FilterMatchesLabelAndDetail(t *testing.T) {
	m := newTestMultiModel([]FilterMultiSelectItem{
		{Label: "MyPlugin", Detail: "custom/plugins/MyPlugin", Value: "MyPlugin"},
		{Label: "OtherPlugin", Detail: "custom/static-plugins/OtherPlugin", Value: "OtherPlugin"},
		{Label: "Swag", Detail: "custom/plugins/Swag", Value: "Swag"},
	})

	m.filter.SetValue("static")
	m.applyFilter()
	assert.Len(t, m.filtered, 1, "filter should match Detail (path)")
	assert.Equal(t, 1, m.filtered[0])

	m.filter.SetValue("swag")
	m.applyFilter()
	assert.Len(t, m.filtered, 1, "filter should match Label")
	assert.Equal(t, 2, m.filtered[0])

	m.filter.SetValue("")
	m.applyFilter()
	assert.Len(t, m.filtered, 3, "empty filter keeps all items")
}

func TestFilterMultiSelect_SpaceTogglesSelection(t *testing.T) {
	m := newTestMultiModel([]FilterMultiSelectItem{
		{Label: "A", Value: "a"},
		{Label: "B", Value: "b"},
		{Label: "C", Value: "c"},
	})

	// Toggle first item.
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	assert.True(t, m.selected[0])

	// Move to third item and toggle it.
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	assert.True(t, m.selected[2])

	// Toggle first item off again.
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	assert.False(t, m.selected[0])

	assert.Len(t, m.selected, 1)
}

func TestFilterMultiSelect_SelectionSurvivesFiltering(t *testing.T) {
	m := newTestMultiModel([]FilterMultiSelectItem{
		{Label: "MyPlugin", Value: "MyPlugin"},
		{Label: "OtherPlugin", Value: "OtherPlugin"},
	})

	// Select first item.
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	assert.True(t, m.selected[0])

	// Filter to hide it, then clear the filter — selection must persist.
	m.filter.SetValue("Other")
	m.applyFilter()
	assert.Len(t, m.filtered, 1)
	m.filter.SetValue("")
	m.applyFilter()
	assert.True(t, m.selected[0], "selection should survive filtering")
}

func TestFilterMultiSelect_EnterConfirms(t *testing.T) {
	m := newTestMultiModel([]FilterMultiSelectItem{{Label: "Only", Value: "only"}})

	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.NotNil(t, cmd, "enter should produce a Quit cmd")
	assert.False(t, m.cancelled)
	assert.True(t, m.selected[0])
}

func TestFilterMultiSelect_EscCancels(t *testing.T) {
	m := newTestMultiModel([]FilterMultiSelectItem{{Label: "Only", Value: "only"}})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.NotNil(t, cmd)
	assert.True(t, m.cancelled)
}
