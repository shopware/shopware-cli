package devtui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewInstanceModel(t *testing.T) {
	m := NewInstanceModel("/tmp/project", true)

	assert.Equal(t, "/tmp/project", m.projectRoot)
	assert.True(t, m.dockerMode)
	assert.True(t, m.follow)
	assert.Equal(t, -1, m.active)
	assert.Equal(t, 0, m.cursor)
	assert.Empty(t, m.sources)
}

// activeLines returns the buffered scrollback of the active source.
func activeLines(m InstanceModel) []string {
	if m.active < 0 || m.active >= len(m.sources) {
		return nil
	}
	return m.sources[m.active].lines
}

func TestInstanceModel_LogLineMsg_AppendsLine(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "web"}}
	m.active = 0

	updated, _ := m.Update(logLineMsg{source: "web", line: "first line"})
	assert.Equal(t, []string{"first line"}, activeLines(updated))

	updated, _ = updated.Update(logLineMsg{source: "web", line: "second line"})
	assert.Equal(t, []string{"first line", "second line"}, activeLines(updated))
}

func TestInstanceModel_LogLineMsg_RoutesToOwningSource(t *testing.T) {
	// A line tagged for a background (non-active) source must land in that
	// source's buffer, not the active one - this is what preserves scrollback
	// while viewing another source.
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "web"}, {name: "worker"}}
	m.active = 0

	updated, _ := m.Update(logLineMsg{source: "worker", line: "bg line"})
	assert.Empty(t, activeLines(updated), "active source buffer should be untouched")
	assert.Equal(t, []string{"bg line"}, updated.sources[1].lines,
		"line should be routed to the owning background source")
}

func TestInstanceModel_LogLineMsg_NoCap(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "web"}}
	m.active = 0

	const count = 1500
	current := m
	for i := 0; i < count; i++ {
		current, _ = current.Update(logLineMsg{source: "web", line: "line"})
	}

	assert.Len(t, activeLines(current), count, "InstanceModel does not currently cap line buffer")
}

func TestInstanceModel_LogLineMsg_FollowAutoScrolls(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 10)
	m.sources = []logSource{{name: "web"}}
	m.active = 0

	current := m
	for i := 0; i < 50; i++ {
		current, _ = current.Update(logLineMsg{source: "web", line: "line"})
	}

	assert.True(t, current.follow)
	assert.True(t, current.viewport.AtBottom(), "follow mode should keep viewport at the bottom")
}

func TestInstanceModel_LogErrMsg_StylesErrorLine(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "web"}}
	m.active = 0

	updated, cmd := m.Update(logErrMsg{err: errors.New("boom")})
	assert.Nil(t, cmd)
	assert.Len(t, activeLines(updated), 1)
	assert.Contains(t, activeLines(updated)[0], "Log stream error: boom")

	expected := errorStyle.Render("Log stream error: boom")
	assert.Equal(t, expected, activeLines(updated)[0])
}

func TestInstanceModel_LogDoneMsg_AppendsTerminator(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "test"}}
	m.active = 0

	updated, _ := m.Update(logDoneMsg{source: "test"})
	assert.Len(t, activeLines(updated), 1)
	assert.Contains(t, activeLines(updated)[0], "log stream ended")
}

func TestInstanceModel_FollowToggle(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	assert.True(t, m.follow)

	fKey := tea.KeyPressMsg(tea.Key{Code: 'f', Text: "f"})

	updated, _ := m.Update(fKey)
	assert.False(t, updated.follow)

	updated, _ = updated.Update(fKey)
	assert.True(t, updated.follow)
}

func TestInstanceModel_SourcesLoadedMsg(t *testing.T) {
	m := NewInstanceModel("/tmp", false)

	sources := []logSource{
		{name: "web"},
		{name: "db"},
	}
	updated, _ := m.Update(logSourcesLoadedMsg{sources: sources})

	assert.Len(t, updated.sources, 2)
	assert.Equal(t, 0, updated.active)
	assert.Equal(t, 0, updated.cursor)
}

func TestInstanceModel_SourcesLoadedMsg_EmptyKeepsInactive(t *testing.T) {
	m := NewInstanceModel("/tmp", false)

	updated, cmd := m.Update(logSourcesLoadedMsg{sources: nil})
	assert.Nil(t, cmd)
	assert.Equal(t, -1, updated.active)
}

func TestInstanceModel_CursorNavigation(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.sources = []logSource{{name: "a"}, {name: "b"}, {name: "c"}}
	m.cursor = 0

	down := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	up := tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})

	updated, _ := m.Update(down)
	assert.Equal(t, 1, updated.cursor)

	updated, _ = updated.Update(down)
	assert.Equal(t, 2, updated.cursor)

	updated, _ = updated.Update(down)
	assert.Equal(t, 2, updated.cursor, "cursor should clamp at last source")

	updated, _ = updated.Update(up)
	assert.Equal(t, 1, updated.cursor)

	updated, _ = updated.Update(up)
	updated, _ = updated.Update(up)
	assert.Equal(t, 0, updated.cursor, "cursor should clamp at zero")
}

func TestInstanceModel_CursorNavigation_FollowsVisualGroupOrder(t *testing.T) {
	// m.sources is in discovery order: containers, then files, then a process
	// added at runtime. The sidebar renders them grouped as containers,
	// processes, files - so arrow keys must walk that visual order, not the
	// raw slice order (otherwise the process appears unreachable).
	m := NewInstanceModel("/tmp", false)
	m.sources = []logSource{
		{name: "web", kind: sourceContainer},
		{name: "worker", kind: sourceContainer},
		{name: "dev.log", kind: sourceFile},
		{name: "Admin Watcher", kind: sourceProcess},
	}
	m.cursor = 1 // "worker", the last container

	down := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	up := tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})

	// Down from the last container should land on the process, not the file.
	updated, _ := m.Update(down)
	assert.Equal(t, "Admin Watcher", updated.sources[updated.cursor].name)

	// Down again reaches the file.
	updated, _ = updated.Update(down)
	assert.Equal(t, "dev.log", updated.sources[updated.cursor].name)

	// Clamp at the last visual item.
	updated, _ = updated.Update(down)
	assert.Equal(t, "dev.log", updated.sources[updated.cursor].name)

	// Up walks back through the process.
	updated, _ = updated.Update(up)
	assert.Equal(t, "Admin Watcher", updated.sources[updated.cursor].name)

	updated, _ = updated.Update(up)
	assert.Equal(t, "worker", updated.sources[updated.cursor].name)
}

func TestInstanceModel_EnterFocusesAndFollows(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "a", filePath: "/nonexistent/a.log"},
		{name: "b", filePath: "/nonexistent/b.log"},
	}
	m.active = 0
	m.cursor = 1
	m.follow = false

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updated, _ := m.Update(enter)

	assert.Equal(t, 1, updated.active)
	assert.True(t, updated.follow)

	updated.StopStreaming()
}

func TestInstanceModel_EnterPreservesEachSourceBuffer(t *testing.T) {
	// Switching between sources must keep each source's own scrollback so
	// returning to a source shows its previous log output.
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "a", lines: []string{"a-1", "a-2"}},
		{name: "b", lines: []string{"b-1"}},
	}
	m.active = 0
	m.cursor = 1

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})

	// Switch to b, then back to a.
	updated, _ := m.Update(enter)
	assert.Equal(t, 1, updated.active)
	assert.Equal(t, []string{"b-1"}, activeLines(updated))

	updated.cursor = 0
	updated, _ = updated.Update(enter)
	assert.Equal(t, 0, updated.active)
	assert.Equal(t, []string{"a-1", "a-2"}, activeLines(updated),
		"source a's scrollback must survive the round trip")

	updated.StopStreaming()
}

func TestInstanceModel_EnterNoChangeWhenCursorEqualsActive(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.sources = []logSource{{name: "a", lines: []string{"keep"}}}
	m.active = 0
	m.cursor = 0

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updated, cmd := m.Update(enter)
	assert.Nil(t, cmd)
	assert.Equal(t, []string{"keep"}, activeLines(updated))
}

func TestInstanceModel_StopStreamingCancelsAllSources(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	aCancelled, bCancelled := false, false
	m.cancels["a"] = func() { aCancelled = true }
	m.cancels["b"] = func() { bCancelled = true }
	m.streaming["a"] = true
	m.streaming["b"] = true

	assert.NotPanics(t, func() {
		m.StopStreaming()
	})

	assert.True(t, aCancelled)
	assert.True(t, bCancelled)
	assert.Empty(t, m.cancels)
	assert.Empty(t, m.streaming)
}

func TestInstanceModel_RemoveSource_DropsActiveAndReselects(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "worker", lines: []string{"w-1"}},
		{name: "Admin Watcher", kind: sourceProcess, lines: []string{"a-1"}},
	}
	m.active = 1
	m.cursor = 1
	cancelled := false
	m.cancels["Admin Watcher"] = func() { cancelled = true }
	m.streaming["Admin Watcher"] = true

	m.RemoveSource("Admin Watcher")

	assert.True(t, cancelled, "the removed source's stream should be cancelled")
	assert.Len(t, m.sources, 1)
	assert.Equal(t, "worker", m.sources[0].name)
	assert.Equal(t, 0, m.active)
	assert.Equal(t, 0, m.cursor)
	assert.NotContains(t, m.cancels, "Admin Watcher")
	assert.NotContains(t, m.streaming, "Admin Watcher")
}

func TestInstanceModel_RemoveSource_KeepsActiveWhenRemovingOther(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "worker"},
		{name: "Admin Watcher", kind: sourceProcess},
		{name: "dev.log", kind: sourceFile},
	}
	m.active = 2 // dev.log
	m.cursor = 2

	m.RemoveSource("Admin Watcher")

	assert.Len(t, m.sources, 2)
	assert.Equal(t, "dev.log", m.sources[m.active].name,
		"active selection should still point at the same source after removal")
	assert.Equal(t, "dev.log", m.sources[m.cursor].name)
}

func TestInstanceModel_RemoveSource_LastSourceGoesInactive(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "Admin Watcher", kind: sourceProcess}}
	m.active = 0
	m.cursor = 0

	m.RemoveSource("Admin Watcher")

	assert.Empty(t, m.sources)
	assert.Equal(t, -1, m.active)
}

func TestInstanceModel_RemoveSource_UnknownIsNoOp(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.sources = []logSource{{name: "worker"}}
	m.active = 0

	assert.NotPanics(t, func() { m.RemoveSource("ghost") })
	assert.Len(t, m.sources, 1)
	assert.Equal(t, 0, m.active)
}

func TestInstanceModel_StopStreaming_NoState(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	assert.NotPanics(t, func() {
		m.StopStreaming()
	})
}

func TestInstanceModel_View_NoSourceSelected(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)

	view := m.View()
	assert.Contains(t, view, "No source selected")
}

func TestInstanceModel_View_ShowsActiveSourceAndLive(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "varnish"}, {name: "mysql"}}
	m.active = 0
	m.cursor = 0

	view := m.View()
	assert.Contains(t, view, "varnish")
	assert.Contains(t, view, "FOLLOWING")
	assert.Contains(t, view, "mysql")
}

func TestInstanceModel_View_RunningProcessUsesHollowDot(t *testing.T) {
	ch := make(chan string)
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "worker", kind: sourceContainer},
		{name: "Admin Watcher", kind: sourceProcess, lineChan: ch},
	}
	m.active = 0 // worker is the active source, not the running process

	view := m.View()
	assert.Contains(t, view, "◦", "a running non-active process should use the hollow dot")
	assert.Contains(t, view, "●", "the active source should use the solid dot")

	// Once the process becomes the active source it takes the solid dot; when
	// no process is active-and-running there should be no leftover hollow dot.
	m.active = 1
	view = m.View()
	assert.NotContains(t, view, "◦",
		"the active process should render the solid dot, not the hollow one")
}

func TestInstanceModel_View_RendersAtVariousSizes(t *testing.T) {
	sizes := []struct {
		w, h int
	}{
		{80, 24},
		{120, 40},
		{200, 60},
		{40, 12},
	}

	for _, sz := range sizes {
		m := NewInstanceModel("/tmp", false)
		m.SetSize(sz.w, sz.h)
		assert.NotPanics(t, func() {
			_ = m.View()
		})
	}
}

func TestInstanceModel_View_NoSourcesShowsHelp(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)

	view := m.View()
	assert.Contains(t, view, "No sources found")
}

func TestInstanceModel_LogLineMsg_RendersInViewport(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "web"}}
	m.active = 0

	updated, _ := m.Update(logLineMsg{source: "web", line: "hello world"})
	content := updated.viewport.GetContent()
	assert.True(t, strings.Contains(content, "hello world"),
		"viewport content should include appended log line, got %q", content)
}

func TestInstanceModel_Init(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	assert.Nil(t, m.Init())
}
