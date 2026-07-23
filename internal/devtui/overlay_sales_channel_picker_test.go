package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/picker"
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
	assert.Equal(t, 2, sp.inner.Len())

	innerNext, _ := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Same(t, sp.inner, innerNext, "down arrow should not dismiss the inner picker")

	innerNext, innerCmd := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, innerNext, "enter should dismiss the inner picker")
	assert.NotNil(t, innerCmd)

	innerMsg := innerCmd()
	pr, ok := innerMsg.(picker.ResultMsg)
	assert.True(t, ok, "inner cmd should produce a picker.ResultMsg, got %T", innerMsg)
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
		overview: NewOverviewModel("local", "http://localhost:8000", "", "", "/tmp/project", nil, nil),
		watchers: make(map[string]*watcherHandle),
	}
	shell := app.New(app.Options{DisableDefaultKeys: true})
	m.host = shell
	shell.SetContent(m)

	sp := newSalesChannelPicker(nil)
	_ = shell.PushOverlay(sp)

	_, _ = shell.Update(salesChannelsLoadedMsg{
		channels: []salesChannelEntry{
			{id: "sc1", name: "Main", domain: "https://main.test", theme: &adminSdk.Theme{Id: "theme-main"}},
		},
	})
	assert.NotNil(t, sp.inner)

	_, cmd := shell.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.NotNil(t, cmd, "enter on the inner picker should yield a cmd")

	innerMsg := cmd()
	pr, ok := innerMsg.(picker.ResultMsg)
	assert.True(t, ok, "expected picker.ResultMsg, got %T", innerMsg)
	_, keyOK := pr.Key.(salesChannelPickerKey)
	assert.True(t, keyOK)

	_, cmd = shell.Update(pr)
	assert.False(t, shell.OverlayOpen(), "overlay should be dismissed after the inner result reaches the outer picker")
	assert.NotNil(t, cmd, "outer picker.ResultMsg handling should yield a cmd")

	outerMsg := cmd()
	res, ok := outerMsg.(salesChannelPickerResultMsg)
	assert.True(t, ok, "expected salesChannelPickerResultMsg, got %T", outerMsg)
	assert.False(t, res.Cancelled)
	assert.Equal(t, "theme-main", res.Opts.ThemeID)

	_, _ = shell.Update(res)
	cur, ok := shell.Content().(Model)
	assert.True(t, ok)
	assert.True(t, cur.overview.sfWatchStarting, "sfWatchStarting must be true after the picker confirms")
}

func TestSalesChannelPicker_ExitWhileLoading(t *testing.T) {
	// Before channels load the user must always have a way out, otherwise the
	// loading/error/empty view feels stuck.
	for _, key := range []rune{tea.KeyEsc, tea.KeyEnter} {
		sp := newSalesChannelPicker(nil)
		assert.Nil(t, sp.inner, "inner picker should not exist before channels load")

		next, cmd := sp.Update(tea.KeyPressMsg(tea.Key{Code: key}))
		assert.Nil(t, next, "%v should dismiss the picker while loading", key)
		assert.NotNil(t, cmd)

		res, ok := cmd().(salesChannelPickerResultMsg)
		assert.True(t, ok)
		assert.True(t, res.Cancelled, "%v while loading should cancel", key)
	}

	// 'q' is also accepted as an exit key.
	sp := newSalesChannelPicker(nil)
	next, cmd := sp.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	assert.Nil(t, next)
	res, ok := cmd().(salesChannelPickerResultMsg)
	assert.True(t, ok)
	assert.True(t, res.Cancelled, "q while loading should cancel")
}
