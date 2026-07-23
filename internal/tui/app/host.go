package app

import (
	tea "charm.land/bubbletea/v2"
)

// Host is the shell surface that Content and Overlay implementations use to
// drive the app without depending on private fields.
//
// *App implements Host. Screens hold a Host so they can open modals, swap
// phases, and run commands:
//
//	type dashboard struct {
//	    host app.Host
//	}
//	func (d *dashboard) openPicker() tea.Cmd {
//	    return d.host.PushOverlay(&picker)
//	}
type Host interface {
	Size() (width, height int)
	Commands() *CommandRegistry
	Keys() *KeyMap
	Status() string
	SetStatus(s string)
	Content() Content
	SetContent(c Content)
	SwapContent(c Content) tea.Cmd
	PushOverlay(o Overlay) tea.Cmd
	PopOverlay()
	OverlayOpen() bool
	RunCommand(id string) tea.Cmd
	RegisterCommand(c Command, keys ...string)
}

// Ensure *App implements Host.
var _ Host = (*App)(nil)

// SwapContent replaces the main content and returns its Init cmd.
// Use this for phase changes (install wizard → dashboard).
func (a *App) SwapContent(c Content) tea.Cmd {
	a.content = c
	if c == nil {
		return nil
	}
	return c.Init()
}
