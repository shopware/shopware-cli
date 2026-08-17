package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

type sizeLeaf struct{ w, h int }

func (l *sizeLeaf) Init() tea.Cmd                         { return nil }
func (l *sizeLeaf) Update(msg tea.Msg) (Content, tea.Cmd) { return l, nil }
func (l *sizeLeaf) View(ctx Context) string               { return "" }
func (l *sizeLeaf) SetSize(w, h int)                      { l.w, l.h = w, h }

type sizeBox struct {
	inner Content
	calls int
}

func (b *sizeBox) Init() tea.Cmd                         { return nil }
func (b *sizeBox) Update(msg tea.Msg) (Content, tea.Cmd) { return b, nil }
func (b *sizeBox) View(ctx Context) string               { return "" }
func (b *sizeBox) PropagateSize(w, h int) {
	b.calls++
	PropagateSize(b.inner, w-2, h-2)
}

func TestPropagateSizeSizeable(t *testing.T) {
	l := &sizeLeaf{}
	PropagateSize(l, 40, 20)
	assert.Equal(t, 40, l.w)
	assert.Equal(t, 20, l.h)
}

func TestPropagateSizeRecursesContainers(t *testing.T) {
	l := &sizeLeaf{}
	b := &sizeBox{inner: l}
	PropagateSize(b, 40, 20)
	assert.Equal(t, 1, b.calls)
	assert.Equal(t, 38, l.w)
	assert.Equal(t, 18, l.h)
}

func TestAppWindowSizePropagates(t *testing.T) {
	l := &sizeLeaf{}
	NewHarness(Options{
		Content: l,
		Header:  func(ctx Context) string { return "H" },
		Footer:  func(ctx Context) string { return "F" },
	}, 40, 12)

	assert.Equal(t, 40, l.w)
	assert.Equal(t, 10, l.h, "12 rows minus header and footer")
}
