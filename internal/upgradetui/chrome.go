package upgradetui

import (
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// chromeRows is the height of the host chrome: the branding header row and
// the shortcut footer row, matching the project dev phase screens.
const chromeRows = 2

// headerView renders the shared branding header as host chrome.
func (m *Model) headerView(ctx app.Context) string {
	return tui.BrandingHeader(ctx.Width)
}

// footerView renders the shared phase footer: the active panel's shortcuts
// plus the exit badge. While an overlay is open it shows only the exit badge —
// the overlay carries its own shortcuts.
func (m *Model) footerView(ctx app.Context) string {
	if ctx.OverlayOpen {
		return tui.PhaseFooter("", "Exit")
	}

	exitLabel := "Exit"
	if m.panel == panelRun && !m.run.finished {
		exitLabel = "Cancel"
	}
	return tui.PhaseFooter(m.footerHint(ctx.Width), exitLabel)
}

// footerHint returns the active panel's shortcut bar.
func (m *Model) footerHint(width int) string {
	fit := width - 20 // room for the exit badge
	switch m.panel {
	case panelIntro:
		return tui.ShortcutBarFit(fit,
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case panelReview:
		return tui.ShortcutBarFit(fit,
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
			tui.Shortcut{Key: "esc", Label: "Back"},
		)
	case panelCheck:
		return tui.ShortcutBarFit(fit,
			tui.Shortcut{Key: "↑/↓", Label: "Select version"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
			tui.Shortcut{Key: "r", Label: "Recheck"},
		)
	case panelPrepare:
		enterLabel := "Details"
		if m.prepare.cursor >= len(m.prepare.results) {
			enterLabel = "Continue"
		}
		return tui.ShortcutBarFit(fit,
			tui.Shortcut{Key: "↑/↓", Label: "Scroll"},
			tui.Shortcut{Key: "enter", Label: enterLabel},
			tui.Shortcut{Key: "e", Label: "Full list"},
			tui.Shortcut{Key: "r", Label: "Recheck"},
			tui.Shortcut{Key: "esc", Label: "Back"},
		)
	case panelRun:
		return tui.ShortcutBarFit(fit, tui.Shortcut{Key: "l", Label: "Toggle full-width log"})
	case panelDone:
		return tui.ShortcutBarFit(fit, tui.Shortcut{Key: "enter", Label: "Close"})
	}
	return ""
}

// windowTitle names the terminal window after the project and wizard.
func (m *Model) windowTitle(app.Context) string {
	return "Upgrade · " + projectName(m.opts.ProjectRoot)
}

// View renders the active panel inside the wizard frame.
func (m *Model) View(ctx app.Context) string {
	m.width = ctx.Width
	m.mainHeight = ctx.MainHeight

	var title, status, body string

	switch m.panel {
	case panelIntro:
		title, status, body = m.viewIntro()
	case panelCheck:
		title, status, body = m.viewCheck()
	case panelPrepare:
		title, status, body = m.viewPrepare()
	case panelReview:
		title, status, body = m.viewReview()
	case panelRun:
		title, status, body = m.viewRun()
	case panelDone:
		title, status, body = m.viewDone()
	}

	return tui.NewWizardFrame(tui.WizardFrameOptions{
		Width:      m.width,
		Height:     m.frameHeight(),
		Title:      title,
		TitleRight: m.contextLabel(),
		Status:     status,
		Body:       body,
	}).Render()
}

func (m *Model) frameHeight() int {
	if m.mainHeight < 8 {
		return 8
	}
	return m.mainHeight
}

// statusStrip renders a status row in the wizard's status slot.
func (m *Model) statusStrip(variant tui.Variant, label, message string) string {
	return tui.NewStatusStrip(tui.StatusStripOptions{Variant: variant, Label: label, Message: message}).Render()
}

// twoColumn renders the standard left/right panel body split.
func (m *Model) twoColumn(leftWidth int, left, right string) string {
	return tui.NewTwoColumn(tui.TwoColumnOptions{
		Width: m.bodyWidth(), LeftWidth: leftWidth, Left: left, Right: right,
	}).Render()
}

// buttonRow renders action buttons in a horizontal row.
func (m *Model) buttonRow(labels []string, active int) string {
	return tui.NewButtonRow(tui.ButtonRowOptions{Labels: labels, Active: active}).Render()
}

// buttonWrap renders action buttons in a row, wrapping within width — for
// the panel's user-action column.
func (m *Model) buttonWrap(width int, labels []string, active int) string {
	return tui.NewButtonRow(tui.ButtonRowOptions{Labels: labels, Active: active, MaxWidth: width}).Render()
}

// rightColumnWidth is the width of the user-action column for a given left
// column width, matching the TwoColumn split.
func (m *Model) rightColumnWidth(leftWidth int) int {
	return m.bodyWidth() - leftWidth - 3
}

// contextLabel is the frame title bar's right side: project, environment, and
// the version path, e.g. "acme-shop · local · Shopware 6.6.10.3 → 6.7.11.0".
func (m *Model) contextLabel() string {
	parts := []string{projectName(m.opts.ProjectRoot)}
	if m.opts.EnvName != "" {
		parts = append(parts, m.opts.EnvName)
	}
	if v := m.versionInfo(); v != "" {
		parts = append(parts, v)
	}
	return strings.Join(parts, " · ")
}

// versionInfo describes the current version and, once selected, the target.
func (m *Model) versionInfo() string {
	target := ""
	if t := m.check.target(); t != nil {
		target = t.Version.String()
	}

	if m.panel == panelDone && m.done.succeeded && target != "" {
		return "Shopware " + target
	}

	if m.check.readiness.CurrentVersion == nil {
		return ""
	}
	current := "Shopware " + m.check.readiness.CurrentVersion.String()
	if target != "" && m.panel != panelIntro && m.panel != panelCheck {
		return current + " → " + target
	}
	return current
}

// bodyWidth returns the inner width available to panel bodies.
func (m *Model) bodyWidth() int {
	w := m.width - 4
	if w < 20 {
		return 20
	}
	return w
}

// bodyHeight returns the rows available to panel bodies inside the frame.
func (m *Model) bodyHeight(hasStatus bool) int {
	h := m.frameHeight() - 4 // frame chrome: borders, title row, rule
	if hasStatus {
		h -= 2
	}
	if h < 4 {
		return 4
	}
	return h
}

// targetLabel renders the "Target 6.7.11.0" hint shown in overlays.
func (m *Model) targetLabel() string {
	if t := m.check.target(); t != nil {
		return "Target " + t.Version.String()
	}
	return ""
}

func projectName(projectRoot string) string {
	return filepath.Base(projectRoot)
}
