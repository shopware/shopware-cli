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
	assert.Empty(t, m.lines)
}

func TestInstanceModel_LogLineMsg_AppendsLine(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)

	updated, _ := m.Update(logLineMsg("first line"))
	assert.Equal(t, []string{"first line"}, updated.lines)

	updated, _ = updated.Update(logLineMsg("second line"))
	assert.Equal(t, []string{"first line", "second line"}, updated.lines)
}

func TestInstanceModel_LogLineMsg_NoCap(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)

	const count = 1500
	current := m
	for i := 0; i < count; i++ {
		current, _ = current.Update(logLineMsg("line"))
	}

	assert.Len(t, current.lines, count, "InstanceModel does not currently cap line buffer")
}

func TestInstanceModel_LogLineMsg_FollowAutoScrolls(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 10)

	current := m
	for i := 0; i < 50; i++ {
		current, _ = current.Update(logLineMsg("line"))
	}

	assert.True(t, current.follow)
	assert.True(t, current.viewport.AtBottom(), "follow mode should keep viewport at the bottom")
}

func TestInstanceModel_LogErrMsg_StylesErrorLine(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)

	updated, cmd := m.Update(logErrMsg{err: errors.New("boom")})
	assert.Nil(t, cmd)
	assert.Len(t, updated.lines, 1)
	assert.Contains(t, updated.lines[0], "Log stream error: boom")

	expected := errorStyle.Render("Log stream error: boom")
	assert.Equal(t, expected, updated.lines[0])
}

func TestInstanceModel_LogDoneMsg_AppendsTerminator(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{{name: "test"}}
	m.active = 0

	updated, cmd := m.Update(logDoneMsg{})
	assert.Nil(t, cmd)
	assert.Len(t, updated.lines, 1)
	assert.Contains(t, updated.lines[0], "log stream ended")
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

func TestInstanceModel_EnterResetsLinesAndFollow(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.sources = []logSource{
		{name: "a", filePath: "/nonexistent/a.log"},
		{name: "b", filePath: "/nonexistent/b.log"},
	}
	m.active = 0
	m.cursor = 1
	m.lines = []string{"stale"}
	m.follow = false

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updated, _ := m.Update(enter)

	assert.Equal(t, 1, updated.active)
	assert.Empty(t, updated.lines)
	assert.True(t, updated.follow)

	updated.StopStreaming()
}

func TestInstanceModel_EnterNoChangeWhenCursorEqualsActive(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.sources = []logSource{{name: "a"}}
	m.active = 0
	m.cursor = 0
	m.lines = []string{"keep"}

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	updated, cmd := m.Update(enter)
	assert.Nil(t, cmd)
	assert.Equal(t, []string{"keep"}, updated.lines)
}

func TestInstanceModel_StopStreamingClearsState(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	ch := make(chan string, 1)
	m.logChan = ch
	cancelCalled := false
	m.cancel = func() { cancelCalled = true }

	assert.NotPanics(t, func() {
		m.StopStreaming()
	})

	assert.True(t, cancelCalled)
	assert.Nil(t, m.cancel)
	assert.Nil(t, m.logChan)
	assert.Nil(t, m.activeProcess)
}

func TestInstanceModel_StopStreaming_NoState(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	assert.NotPanics(t, func() {
		m.StopStreaming()
	})
}

func TestInstanceModel_ActiveProcessSourceName(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	assert.Equal(t, "", m.ActiveProcessSourceName())

	m.sources = []logSource{{name: "file-only", filePath: "/tmp/x.log"}}
	m.active = 0
	assert.Equal(t, "", m.ActiveProcessSourceName(),
		"sources without a Process should not be reported as process sources")
}

func TestInstanceModel_View_FollowBadgeOn(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.follow = true

	var view string
	assert.NotPanics(t, func() {
		view = m.View()
	})
	assert.Contains(t, view, "FOLLOW ON")
	assert.NotContains(t, view, "FOLLOW OFF")
}

func TestInstanceModel_View_FollowBadgeOff(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	m.SetSize(120, 40)
	m.follow = false

	view := m.View()
	assert.Contains(t, view, "FOLLOW OFF")
	assert.NotContains(t, view, "FOLLOW ON")
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
	assert.Contains(t, view, "LIVE")
	assert.Contains(t, view, "mysql")
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

	updated, _ := m.Update(logLineMsg("hello world"))
	content := updated.viewport.GetContent()
	assert.True(t, strings.Contains(content, "hello world"),
		"viewport content should include appended log line, got %q", content)
}

func TestInstanceModel_Init(t *testing.T) {
	m := NewInstanceModel("/tmp", false)
	assert.Nil(t, m.Init())
}
