package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
)

func TestSalesChannelPicker_ConfirmEmitsWatcherOpts(t *testing.T) {
	sp := newSalesChannelPicker(nil)

	loaded := salesChannelsLoadedMsg{
		channels: []salesChannelEntry{
			{id: "sc1", name: "Storefront EU", domain: "https://eu.example.test", theme: &adminSdk.Theme{Id: "theme-eu"}},
			{id: "sc2", name: "Storefront US", domain: "https://us.example.test", theme: &adminSdk.Theme{Id: "theme-us"}},
		},
	}

	next, _ := sp.Update(loaded)
	sp, ok := next.(*salesChannelPicker)
	assert.True(t, ok)
	assert.NotNil(t, sp.inner, "inner listPicker should be created after channels load")
	assert.Len(t, sp.inner.items, 2)

	innerNext, _ := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Same(t, sp.inner, innerNext, "down arrow should not dismiss the inner picker")

	innerNext, innerCmd := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, innerNext, "enter should dismiss the inner picker")
	assert.NotNil(t, innerCmd)

	innerMsg := innerCmd()
	pr, ok := innerMsg.(pickerResultMsg)
	assert.True(t, ok, "inner cmd should produce a pickerResultMsg, got %T", innerMsg)
	assert.False(t, pr.Cancelled)
	assert.Equal(t, 1, pr.Index)
	_, keyOK := pr.Key.(salesChannelPickerKey)
	assert.True(t, keyOK)

	outerNext, outerCmd := sp.Update(pr)
	assert.Nil(t, outerNext, "outer picker should dismiss after a result")
	assert.NotNil(t, outerCmd)

	outerMsg := outerCmd()
	res, ok := outerMsg.(salesChannelPickerResultMsg)
	assert.True(t, ok, "outer cmd should produce a salesChannelPickerResultMsg, got %T", outerMsg)
	assert.False(t, res.Cancelled)
	assert.Equal(t, "theme-us", res.Opts.ThemeID)
	assert.Equal(t, "https://us.example.test", res.Opts.DomainURL)
}

func TestModel_SalesChannelPicker_FullRoutingFlow(t *testing.T) {
	m := Model{
		phase:    phaseDashboard,
		overview:  NewOverviewModel("local", "http://localhost:8000", "", "", "/tmp/project", nil, nil),
		watchers: make(map[string]*watcherHandle),
	}

	sp := newSalesChannelPicker(nil)
	m.modal = sp

	next, _ := sp.Update(salesChannelsLoadedMsg{
		channels: []salesChannelEntry{
			{id: "sc1", name: "Main", domain: "https://main.test", theme: &adminSdk.Theme{Id: "theme-main"}},
		},
	})
	sp = next.(*salesChannelPicker)
	m.modal = sp
	assert.NotNil(t, sp.inner)

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)
	assert.NotNil(t, cmd, "enter on the inner picker should yield a cmd")

	innerMsg := cmd()
	pr, ok := innerMsg.(pickerResultMsg)
	assert.True(t, ok, "expected pickerResultMsg, got %T", innerMsg)
	_, keyOK := pr.Key.(salesChannelPickerKey)
	assert.True(t, keyOK)

	updated, cmd = m.Update(pr)
	m = updated.(Model)
	assert.Nil(t, m.modal, "modal should be dismissed after the inner result reaches the outer picker")
	assert.NotNil(t, cmd, "outer pickerResultMsg handling should yield a cmd")

	outerMsg := cmd()
	res, ok := outerMsg.(salesChannelPickerResultMsg)
	assert.True(t, ok, "expected salesChannelPickerResultMsg, got %T", outerMsg)
	assert.False(t, res.Cancelled)
	assert.Equal(t, "theme-main", res.Opts.ThemeID)

	updated, _ = m.Update(res)
	m = updated.(Model)
	assert.True(t, m.overview.sfWatchStarting, "sfWatchStarting must be true after the picker confirms")
}
