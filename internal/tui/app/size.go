package app

import (
	tea "charm.land/bubbletea/v2"
)

// Sizeable content can receive terminal size updates when chrome is measured.
//
//	func (d *dashboard) SetSize(w, h int) {
//	    d.listW, d.listH = w/3, h
//	}
type Sizeable interface {
	SetSize(width, height int)
}

// SizePropagator is implemented by container Content (tabs, split, phases, …)
// to recurse size into children. Keeps the host free of concrete container types.
type SizePropagator interface {
	PropagateSize(width, height int)
}

// NotifySize calls SetSize if c implements Sizeable (non-recursive).
// Prefer PropagateSize for app content trees.
func NotifySize(c Content, width, height int) {
	if c == nil || width < 1 || height < 1 {
		return
	}
	if s, ok := c.(Sizeable); ok {
		s.SetSize(width, height)
	}
}

// PropagateSize notifies Sizeable and SizePropagator along the content tree.
// Container content implements SizePropagator to recurse into children.
func PropagateSize(c Content, width, height int) {
	if c == nil || width < 1 || height < 1 {
		return
	}
	if s, ok := c.(Sizeable); ok {
		s.SetSize(width, height)
	}
	if p, ok := c.(SizePropagator); ok {
		p.PropagateSize(width, height)
	}
}

// ApplySizeMsg is a convenience for Content.Update to react to WindowSizeMsg
// after chrome has already been accounted for via Context.MainHeight.
func ApplySizeMsg(c Content, msg tea.Msg, width, mainHeight int) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		PropagateSize(c, width, mainHeight)
	}
}
