package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
)

// Drives the salesChannelPicker through the load → list → confirm flow and
// verifies it emits a salesChannelPickerResultMsg carrying the chosen theme
// and domain. Catches regressions where the inner listPicker's pickerResultMsg
// fails to translate into the outer message that triggers the watcher.
//
// Mirrors the Model.Update routing for pickerResultMsg: when the inner
// pickerResultMsg arrives, the Model breaks out of its switch and forwards
// the message to m.modal. This test simulates that handoff.
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

	// Move cursor to second channel and confirm via the inner picker.
	innerNext, _ := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Same(t, sp.inner, innerNext, "down arrow should not dismiss the inner picker")

	innerNext, innerCmd := sp.inner.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, innerNext, "enter should dismiss the inner picker")
	assert.NotNil(t, innerCmd)

	// The inner emits a pickerResultMsg via the cmd; surface it.
	innerMsg := innerCmd()
	pr, ok := innerMsg.(pickerResultMsg)
	assert.True(t, ok, "inner cmd should produce a pickerResultMsg, got %T", innerMsg)
	assert.False(t, pr.Cancelled)
	assert.Equal(t, 1, pr.Index)
	_, keyOK := pr.Key.(salesChannelPickerKey)
	assert.True(t, keyOK)

	// Outer picker translates the inner result into the watcher-options message.
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

// End-to-end flow through Model.Update: simulates pressing Enter on a sales
// channel and verifies the model ends up with sfWatchStarting=true and a
// non-nil cmd to start the watcher. This guards the full routing chain:
//  1. Inner listPicker emits pickerResultMsg via cmd
//  2. Model.Update breaks on pickerResultMsg and forwards to modal
//  3. salesChannelPicker translates it to salesChannelPickerResultMsg
//  4. Model.Update applies sfWatchStarting and returns the watcher cmd
func TestModel_SalesChannelPicker_FullRoutingFlow(t *testing.T) {
	m := Model{
		phase:    phaseDashboard,
		general:  NewGeneralModel("local", "http://localhost:8000", "", "", "/tmp/project", nil),
		watchers: make(map[string]*executor.Process),
	}

	sp := newSalesChannelPicker(nil)
	m.modal = sp

	// Pre-populate channels (skipping the async load).
	next, _ := sp.Update(salesChannelsLoadedMsg{
		channels: []salesChannelEntry{
			{id: "sc1", name: "Main", domain: "https://main.test", theme: &adminSdk.Theme{Id: "theme-main"}},
		},
	})
	sp = next.(*salesChannelPicker)
	m.modal = sp
	assert.NotNil(t, sp.inner)

	// User confirms with Enter on the only item.
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)
	assert.NotNil(t, cmd, "enter on the inner picker should yield a cmd")

	// Drain the cmd; it produces pickerResultMsg from the inner listPicker.
	innerMsg := cmd()
	pr, ok := innerMsg.(pickerResultMsg)
	assert.True(t, ok, "expected pickerResultMsg, got %T", innerMsg)
	_, keyOK := pr.Key.(salesChannelPickerKey)
	assert.True(t, keyOK)

	// Feed the pickerResultMsg back through Model.Update — it should reach the
	// sales-channel picker and emit a salesChannelPickerResultMsg.
	updated, cmd = m.Update(pr)
	m = updated.(Model)
	assert.Nil(t, m.modal, "modal should be dismissed after the inner result reaches the outer picker")
	assert.NotNil(t, cmd, "outer pickerResultMsg handling should yield a cmd")

	outerMsg := cmd()
	res, ok := outerMsg.(salesChannelPickerResultMsg)
	assert.True(t, ok, "expected salesChannelPickerResultMsg, got %T", outerMsg)
	assert.False(t, res.Cancelled)
	assert.Equal(t, "theme-main", res.Opts.ThemeID)

	// Feed salesChannelPickerResultMsg back — model should mark watcher as starting.
	updated, _ = m.Update(res)
	m = updated.(Model)
	assert.True(t, m.general.sfWatchStarting, "sfWatchStarting must be true after the picker confirms")
}
