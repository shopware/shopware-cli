package app

import (
	tea "charm.land/bubbletea/v2"
)

// Content is the main screen body hosted inside App.
// Contents return string views; App wraps them in chrome + tea.View.
type Content interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Content, tea.Cmd)
	View(ctx Context) string
}

// ContentFunc adapts functions to Content.
type ContentFunc struct {
	OnInit   func() tea.Cmd
	OnUpdate func(msg tea.Msg) (Content, tea.Cmd)
	OnView   func(ctx Context) string
}

// Init implements Content.
func (c ContentFunc) Init() tea.Cmd {
	if c.OnInit != nil {
		return c.OnInit()
	}
	return nil
}

// Update implements Content.
func (c ContentFunc) Update(msg tea.Msg) (Content, tea.Cmd) {
	if c.OnUpdate != nil {
		return c.OnUpdate(msg)
	}
	return c, nil
}

// View implements Content.
func (c ContentFunc) View(ctx Context) string {
	if c.OnView != nil {
		return c.OnView(ctx)
	}
	return ""
}

// StaticContent shows a fixed string (useful for tests and placeholders).
type StaticContent struct {
	Text string
}

// Init implements Content.
func (s StaticContent) Init() tea.Cmd { return nil }

// Update implements Content.
func (s StaticContent) Update(tea.Msg) (Content, tea.Cmd) { return s, nil }

// View implements Content.
func (s StaticContent) View(Context) string { return s.Text }
