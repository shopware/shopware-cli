package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Overlay is a modal layer that captures input until it closes.
//
// Contract:
//   - Update returns the next overlay; nil means "closed"
//   - Results are delivered with Emit(resultMsg) so the parent handles outcomes
//   - View receives the terminal size for full-screen layout
type Overlay interface {
	// Init is called once when the overlay is pushed.
	Init() tea.Cmd
	// Update handles a message. Returning a nil overlay closes it.
	// Prefer Emit(result) for outcomes instead of getters after close.
	Update(msg tea.Msg) (next Overlay, cmd tea.Cmd)
	// View renders the overlay for the given terminal size (full screen).
	View(width, height int) string
	// ID optionally names the overlay for debugging; may be empty.
	ID() string
}

// OverlayStack is a LIFO stack of overlays. The top overlay receives input.
type OverlayStack struct {
	stack []Overlay
}

// Len returns the number of open overlays.
func (s *OverlayStack) Len() int {
	if s == nil {
		return 0
	}
	return len(s.stack)
}

// Open reports whether any overlay is showing.
func (s *OverlayStack) Open() bool {
	return s.Len() > 0
}

// Push adds an overlay on top and returns its Init cmd.
func (s *OverlayStack) Push(o Overlay) tea.Cmd {
	if s == nil || o == nil {
		return nil
	}
	s.stack = append(s.stack, o)
	return o.Init()
}

// Pop removes the top overlay if any.
func (s *OverlayStack) Pop() {
	if s == nil || len(s.stack) == 0 {
		return
	}
	s.stack = s.stack[:len(s.stack)-1]
}

// Top returns the top overlay, or nil.
func (s *OverlayStack) Top() Overlay {
	if s == nil || len(s.stack) == 0 {
		return nil
	}
	return s.stack[len(s.stack)-1]
}

// Update routes a message to the top overlay. If the overlay returns nil, it
// is popped and closed reports true.
func (s *OverlayStack) Update(msg tea.Msg) (cmd tea.Cmd, closed bool) {
	if s == nil || len(s.stack) == 0 {
		return nil, false
	}
	i := len(s.stack) - 1
	next, c := s.stack[i].Update(msg)
	if next == nil {
		s.stack = s.stack[:i]
		return c, true
	}
	s.stack[i] = next
	return c, false
}

// View returns the top overlay view for the given size, or empty if none.
func (s *OverlayStack) View(width, height int) string {
	top := s.Top()
	if top == nil {
		return ""
	}
	return top.View(width, height)
}

// FuncOverlay adapts functions into an Overlay (handy for quick dialogs).
type FuncOverlay struct {
	Name string
	// OnInit optional.
	OnInit func() tea.Cmd
	// OnUpdate required. Return next=nil to close.
	OnUpdate func(msg tea.Msg) (next Overlay, cmd tea.Cmd)
	// OnView required; receives the terminal size.
	OnView func(width, height int) string
}

// Init implements Overlay.
func (f *FuncOverlay) Init() tea.Cmd {
	if f.OnInit != nil {
		return f.OnInit()
	}
	return nil
}

// Update implements Overlay.
func (f *FuncOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if f.OnUpdate == nil {
		return nil, nil
	}
	return f.OnUpdate(msg)
}

// View implements Overlay.
func (f *FuncOverlay) View(width, height int) string {
	if f.OnView == nil {
		return ""
	}
	return f.OnView(width, height)
}

// ID implements Overlay.
func (f *FuncOverlay) ID() string { return f.Name }

// CenterPanel places content centered in width×height, for overlays that
// replace the whole screen.
func CenterPanel(width, height int, content string) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
