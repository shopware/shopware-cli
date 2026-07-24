package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestStatusStrip(t *testing.T) {
	strip := NewStatusStrip(StatusStripOptions{
		Variant: VariantError, Label: "BLOCKED", Message: "3 extensions need fixes before the upgrade can continue.",
	}).Render()

	plain := ansi.Strip(strip)
	assert.Contains(t, plain, "BLOCKED")
	assert.Contains(t, plain, "3 extensions need fixes")

	// Label column is padded so messages align across strips of different kinds.
	ready := ansi.Strip(NewStatusStrip(StatusStripOptions{
		Variant: VariantSuccess, Label: "READY", Message: "all good",
	}).Render())
	assert.Equal(t, strings.Index(plain, "3 extensions"), strings.Index(ready, "all good"))
}

func TestTwoColumn(t *testing.T) {
	out := NewTwoColumn(TwoColumnOptions{
		Width: 60, LeftWidth: 30,
		Left:  "left line 1\nleft line 2\nleft line 3",
		Right: "right line 1",
	}).Render()

	lines := strings.Split(out, "\n")
	assert.Len(t, lines, 3, "row count is the max of both columns")
	for _, line := range lines {
		assert.Equal(t, 60, lipgloss.Width(line), "every row is padded to the full width")
	}
	assert.Contains(t, ansi.Strip(lines[0]), "left line 1")
	assert.Contains(t, ansi.Strip(lines[0]), "│")
	assert.Contains(t, ansi.Strip(lines[0]), "right line 1")
	assert.Contains(t, ansi.Strip(lines[2]), "left line 3")
}

func TestTwoColumnTruncatesOverflow(t *testing.T) {
	out := NewTwoColumn(TwoColumnOptions{
		Width: 40, LeftWidth: 20, Left: strings.Repeat("x", 50), Right: "right",
	}).Render()

	lines := strings.Split(out, "\n")
	assert.Equal(t, 40, lipgloss.Width(lines[0]))
	assert.Contains(t, ansi.Strip(lines[0]), "…")
}

func TestWizardFrame(t *testing.T) {
	status := NewStatusStrip(StatusStripOptions{Variant: VariantInfo, Label: "TODO", Message: "Review the plan."}).Render()
	frame := NewWizardFrame(WizardFrameOptions{
		Width: 80, Height: 20,
		Title: "Upgrade Shopware to a newer version", TitleRight: "Target 6.7.11.0",
		Status: status, Body: "body content", Footer: "Begin upgrade  Cancel",
	}).Render()

	lines := strings.Split(frame, "\n")
	assert.Len(t, lines, 20, "frame fills the requested height exactly")
	for _, line := range lines {
		assert.Equal(t, 80, lipgloss.Width(line), "every row spans the full width")
	}

	plain := ansi.Strip(frame)
	assert.Contains(t, plain, "Upgrade Shopware to a newer version")
	assert.Contains(t, plain, "Target 6.7.11.0")
	assert.Contains(t, plain, "TODO")
	assert.Contains(t, plain, "body content")
	assert.Contains(t, plain, "Begin upgrade")
	assert.True(t, strings.HasPrefix(plain, "╭"))
	assert.True(t, strings.HasSuffix(strings.TrimRight(plain, "\n"), "╯"))
	// Title, status, and footer rows are separated by horizontal rules.
	assert.Equal(t, 3, strings.Count(plain, "├"))
}

func TestWizardFrameWithoutStatus(t *testing.T) {
	frame := NewWizardFrame(WizardFrameOptions{
		Width: 60, Height: 12, Title: "Title", Body: "body", Footer: "footer",
	}).Render()

	lines := strings.Split(frame, "\n")
	assert.Len(t, lines, 12)
	assert.Equal(t, 2, strings.Count(ansi.Strip(frame), "├"), "only title and footer rules")
}

func TestScrollbar(t *testing.T) {
	fits := NewScrollbar(ScrollbarOptions{Total: 5, Visible: 10, Offset: 0, Height: 8}).Render()
	assert.Empty(t, fits, "no scrollbar when everything fits")

	bar := NewScrollbar(ScrollbarOptions{Total: 100, Visible: 10, Offset: 0, Height: 10}).Render()
	lines := strings.Split(bar, "\n")
	assert.Len(t, lines, 10)
	plain := ansi.Strip(bar)
	assert.True(t, strings.HasPrefix(plain, "↑"))
	assert.True(t, strings.HasSuffix(plain, "↓"))
	assert.Contains(t, plain, "█")

	// Thumb moves to the bottom when scrolled to the end.
	end := ansi.Strip(NewScrollbar(ScrollbarOptions{Total: 100, Visible: 10, Offset: 90, Height: 10}).Render())
	endLines := strings.Split(end, "\n")
	assert.Equal(t, "█", endLines[len(endLines)-2])
}

func TestButtonRow(t *testing.T) {
	row := NewButtonRow(ButtonRowOptions{Labels: []string{"Continue", "Recheck", "Cancel"}, Active: 1}).Render()
	plain := ansi.Strip(row)
	assert.Contains(t, plain, "Continue")
	assert.Contains(t, plain, "Recheck")
	assert.Contains(t, plain, "Cancel")

	first := NewButtonRow(ButtonRowOptions{Labels: []string{"A", "B"}, Active: 0}).Render()
	second := NewButtonRow(ButtonRowOptions{Labels: []string{"A", "B"}, Active: 1}).Render()
	assert.NotEqual(t, first, second, "focus changes rendering")

	assert.Equal(t, first, ConfirmButtons("A", "B", true), "ConfirmButtons helper matches the component")
}

func TestButtonRowWrap(t *testing.T) {
	labels := []string{"Start upgrade", "Export report", "Back", "Cancel"}

	// Wide enough: everything stays on one row.
	wide := NewButtonRow(ButtonRowOptions{Labels: labels, Active: 0, MaxWidth: 80}).Render()
	assert.Equal(t, 1, len(strings.Split(wide, "\n")))

	// Narrow: buttons wrap onto multiple rows separated by blank lines,
	// and every row fits the limit.
	narrow := NewButtonRow(ButtonRowOptions{Labels: labels, Active: 0, MaxWidth: 40}).Render()
	rows := strings.Split(narrow, "\n\n")
	assert.Greater(t, len(rows), 1)
	for _, row := range rows {
		assert.LessOrEqual(t, lipgloss.Width(row), 40)
	}
	plain := ansi.Strip(narrow)
	for _, label := range labels {
		assert.Contains(t, plain, label)
	}
}

func TestModal(t *testing.T) {
	modal := NewModal(ModalOptions{MaxWidth: 40, AreaWidth: 80, AreaHeight: 12})
	assert.Equal(t, 40, modal.Width())
	assert.Equal(t, 34, modal.ContentWidth(), "border and padding subtracted")

	out := modal.Render("hello modal")
	lines := strings.Split(out, "\n")
	assert.Len(t, lines, 12, "modal fills the area height")
	for _, line := range lines {
		assert.Equal(t, 80, lipgloss.Width(line), "centered rows span the area width")
	}
	plain := ansi.Strip(out)
	assert.Contains(t, plain, "hello modal")
	assert.Contains(t, plain, "╭")
	assert.Contains(t, plain, "╰")

	// Narrow areas shrink the box below MaxWidth.
	narrow := NewModal(ModalOptions{MaxWidth: 40, AreaWidth: 30, AreaHeight: 6})
	assert.Equal(t, 26, narrow.Width())
}

func TestStateDotAndCheckRow(t *testing.T) {
	assert.Equal(t, "●", ansi.Strip(StateDot(DotOK)))
	assert.Equal(t, "◐", ansi.Strip(StateDot(DotRunning)))
	assert.Equal(t, "○", ansi.Strip(StateDot(DotPending)))

	row := NewCheckRow(CheckRowOptions{State: DotOK, Label: "Git working tree clean", Value: "yes", LabelWidth: 30}).Render()
	plain := ansi.Strip(row)
	assert.Equal(t, "● Git working tree clean        yes", plain)

	other := ansi.Strip(NewCheckRow(CheckRowOptions{State: DotError, Label: "composer.lock", Value: "no", LabelWidth: 30}).Render())
	assert.Equal(t, strings.Index(plain, "yes"), strings.Index(other, "no"), "value columns align")
}

func TestStepList(t *testing.T) {
	list := NewStepList(StepListOptions{Steps: []StepItem{
		{Label: "done step", State: StepStateDone},
		{Label: "active step", State: StepStateActive, Indicator: "◐"},
		{Label: "pending step", State: StepStatePending},
	}}).Render()

	plain := ansi.Strip(list)
	assert.Contains(t, plain, "✓ done step")
	assert.Contains(t, plain, "◐ active step")
	assert.Contains(t, plain, "· pending step")
}

func TestShortcutsFit(t *testing.T) {
	items := []Shortcut{
		{Key: "↑/↓", Label: "Navigate"},
		{Key: "enter", Label: "Select"},
		{Key: "ctrl+c", Label: "Exit"},
	}

	wide := NewShortcuts(ShortcutsOptions{Items: items}).Render()
	assert.Equal(t, wide, ShortcutBar(items...), "ShortcutBar helper matches the component")

	narrow := NewShortcuts(ShortcutsOptions{Items: items, MaxWidth: 20}).Render()
	assert.LessOrEqual(t, lipgloss.Width(narrow), 20)
	assert.Equal(t, narrow, ShortcutBarFit(20, items...))
}
