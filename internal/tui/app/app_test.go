package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

func ctrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
}

// testOverlay closes on esc, emitting a result.
type testOverlay struct {
	inited bool
	seen   []tea.Msg
}

type overlayResultMsg struct{}

func (o *testOverlay) Init() tea.Cmd { o.inited = true; return nil }
func (o *testOverlay) ID() string    { return "test" }

func (o *testOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	o.seen = append(o.seen, msg)
	if key, ok := msg.(tea.KeyPressMsg); ok && KeyString(key) == "esc" {
		return nil, Emit(overlayResultMsg{})
	}
	return o, nil
}

func (o *testOverlay) View(width, height int) string { return "OVERLAY" }

func TestFrameLayout(t *testing.T) {
	out := Frame(20, 6, "header", "line1\nline2", "footer")
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 6)
	assert.Equal(t, "header", lines[0])
	assert.Equal(t, "line1", lines[1])
	assert.Equal(t, "line2", lines[2])
	assert.Equal(t, "", lines[3], "main is padded to fill")
	assert.Equal(t, "footer", lines[5])
}

func TestFrameTruncatesOverflowingMain(t *testing.T) {
	out := Frame(10, 3, "h", "1\n2\n3\n4\n5", "f")
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"h", "1", "f"}, lines)
}

func TestComputeRegionClampsChrome(t *testing.T) {
	r := ComputeRegion(10, 3, "a\nb\nc", "x\ny\nz")
	assert.Equal(t, 3, r.Height)
	assert.GreaterOrEqual(t, r.Main, 1, "main survives oversized chrome")
	assert.Equal(t, r.Height, r.Header+r.Main+r.Footer)
}

func TestOverlayStackNilCloseAndResult(t *testing.T) {
	var stack OverlayStack
	assert.False(t, stack.Open())

	o := &testOverlay{}
	stack.Push(o)
	assert.True(t, o.inited, "push runs Init")
	assert.True(t, stack.Open())

	cmd, closed := stack.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.True(t, closed, "returning nil pops the overlay")
	assert.False(t, stack.Open())
	require.NotNil(t, cmd)
	assert.IsType(t, overlayResultMsg{}, cmd(), "result is emitted as a command")
}

func TestAppQuitKey(t *testing.T) {
	h := NewHarness(Options{Content: StaticContent{Text: "hello"}}, 40, 10)

	cmd := h.Send(ctrlC())
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestAppQuitCommandOverride(t *testing.T) {
	intercepted := false
	h := NewHarness(Options{Content: StaticContent{Text: "hello"}}, 40, 10)
	h.App.RegisterCommand(Command{
		ID: CmdQuit,
		Run: func(*App) tea.Cmd {
			intercepted = true
			return nil
		},
	})

	cmd := h.Send(ctrlC())
	assert.True(t, intercepted)
	assert.Nil(t, cmd, "an overridden quit command returning nil swallows the quit")
}

func TestAppKeyBindingRunsCommand(t *testing.T) {
	ran := 0
	h := NewHarness(Options{
		Content:  StaticContent{Text: "hello"},
		Bindings: []Binding{{Keys: []string{"r"}, Command: "test.run"}},
	}, 40, 10)
	h.App.Commands().Register(Command{ID: "test.run", Run: func(*App) tea.Cmd { ran++; return nil }})

	h.Send(keyPress('r'))
	assert.Equal(t, 1, ran)

	// Non-global bindings do not fire while an overlay is open.
	h.App.PushOverlay(&testOverlay{})
	h.Send(keyPress('r'))
	assert.Equal(t, 1, ran, "overlay captures the bound key")
}

func TestAppChromeAndContent(t *testing.T) {
	h := NewHarness(Options{
		Content: StaticContent{Text: "body"},
		Header:  func(ctx Context) string { return "HEADER" },
		Footer:  func(ctx Context) string { return "FOOTER" },
		WindowTitleFunc: func(ctx Context) string {
			return "my-title"
		},
	}, 40, 8)

	view := h.View()
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 8, "frame fills the terminal height")
	assert.Equal(t, "HEADER", lines[0])
	assert.Equal(t, "body", lines[1])
	assert.Equal(t, "FOOTER", lines[7])

	assert.Equal(t, "my-title", h.App.View().WindowTitle)
}

func TestAppContextMainHeight(t *testing.T) {
	h := NewHarness(Options{
		Content: StaticContent{Text: "body"},
		Header:  func(ctx Context) string { return "H1\nH2" },
		Footer:  func(ctx Context) string { return "F" },
	}, 40, 10)

	ctx := h.App.Context()
	assert.Equal(t, 10, ctx.Height)
	assert.Equal(t, 7, ctx.MainHeight, "10 rows minus 2 header minus 1 footer")
}

func TestAppOverlayCapturesInput(t *testing.T) {
	recorded := ""
	var self Content
	self = ContentFunc{
		OnUpdate: func(msg tea.Msg) (Content, tea.Cmd) {
			if key, ok := msg.(tea.KeyPressMsg); ok {
				recorded += KeyString(key)
			}
			return self, nil
		},
		OnView: func(ctx Context) string { return "CONTENT" },
	}

	h := NewHarness(Options{Content: self}, 40, 10)
	overlay := &testOverlay{}
	h.App.PushOverlay(overlay)

	assert.Contains(t, h.View(), "OVERLAY", "fullscreen overlay replaces the frame")

	h.Send(keyPress('x'))
	assert.Empty(t, recorded, "keys go to the overlay, not content")
	require.NotEmpty(t, overlay.seen)

	// Non-input messages reach both overlay and content.
	type asyncMsg struct{}
	h.Send(asyncMsg{})
	assert.Contains(t, h.View(), "OVERLAY")

	// esc closes; next key reaches content again.
	h.Send(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	assert.False(t, h.App.OverlayOpen())
	h.Send(keyPress('y'))
	assert.Equal(t, "y", recorded)
}

func TestAppSwapContent(t *testing.T) {
	inited := false
	first := StaticContent{Text: "first"}
	second := ContentFunc{
		OnInit: func() tea.Cmd { inited = true; return nil },
		OnView: func(ctx Context) string { return "second" },
	}

	h := NewHarness(Options{Content: first}, 40, 5)
	assert.Contains(t, h.View(), "first")

	_ = h.App.SwapContent(second)
	assert.True(t, inited, "SwapContent runs Init")
	assert.Contains(t, h.View(), "second")
}
