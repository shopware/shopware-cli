package pluginmigrate

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	migrate "github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// chromeRows is the height the shell's header and footer occupy.
const chromeRows = 2

func (m *Model) headerView(ctx app.Context) string {
	return m.header.View(ctx.Width)
}

func (m *Model) windowTitle(app.Context) string {
	return "Autofix · " + projectName(m.opts.ProjectRoot)
}

func (m *Model) footerView(app.Context) string {
	exitLabel := "Exit"
	var hint string

	switch m.panel {
	case panelWelcome:
		hint = tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case panelToken:
		hint = tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Focus"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
			tui.Shortcut{Key: "esc", Label: "Back"},
		)
	case panelReview:
		hint = tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
			tui.Shortcut{Key: "esc", Label: "Back"},
		)
	case panelRun:
		if !m.run.finished {
			exitLabel = "Cancel"
		}
	case panelDone:
		hint = tui.ShortcutBar(tui.Shortcut{Key: "enter", Label: "Close"})
	}

	return tui.PhaseFooter(hint, exitLabel)
}

// View renders the main region: mascot cards for welcome/done, the wizard
// frame for the working panels.
func (m *Model) View(ctx app.Context) string {
	m.width = ctx.Width
	m.mainHeight = ctx.MainHeight
	if ctx.Width == 0 || ctx.Height == 0 {
		return ""
	}

	switch m.panel {
	case panelWelcome:
		return m.centerCard(m.viewWelcome())
	case panelDone:
		return m.centerCard(m.viewDone())
	case panelToken, panelReview, panelRun:
	}

	var title, status, body string
	switch m.panel { //nolint:exhaustive
	case panelToken:
		title, status, body = m.viewToken()
	case panelReview:
		title, status, body = m.viewReview()
	case panelRun:
		title, status, body = m.viewRun()
	}

	return tui.NewWizardFrame(tui.WizardFrameOptions{
		Width:      m.width,
		Height:     m.frameHeight(),
		Title:      title,
		TitleRight: projectName(m.opts.ProjectRoot),
		Status:     status,
		Body:       body,
	}).Render()
}

func (m *Model) centerCard(card string) string {
	return lipgloss.Place(max(m.width, 1), max(m.mainHeight, 1), lipgloss.Center, lipgloss.Center, card)
}

func (m *Model) frameHeight() int {
	if m.mainHeight < 8 {
		return 8
	}
	return m.mainHeight
}

// bodyWidth returns the inner width available to panel bodies.
func (m *Model) bodyWidth() int {
	w := m.width - 4
	if w < 20 {
		return 20
	}
	return w
}

// statusStrip renders a status row in the wizard's status slot.
func (m *Model) statusStrip(variant tui.Variant, label, message string) string {
	return tui.NewStatusStrip(tui.StatusStripOptions{Variant: variant, Label: label, Message: message}).Render()
}

// summaryCounts describes the scan result, e.g.
// "3 extensions in custom/ — 2 on the Shopware Store, 1 local".
func (m *Model) summaryCounts() string {
	if len(m.plan.Extensions) == 0 {
		return fmt.Sprintf("%d extensions in custom/", len(m.scan))
	}
	details := []string{}
	if n := m.plan.Count(migrate.ActionStoreRequire); n > 0 {
		details = append(details, fmt.Sprintf("%d on the Shopware Store", n))
	}
	if n := m.plan.Count(migrate.ActionComposerRequire); n > 0 {
		details = append(details, fmt.Sprintf("%d on Packagist", n))
	}
	if n := m.plan.Count(migrate.ActionPathRepository); n > 0 {
		details = append(details, fmt.Sprintf("%d local", n))
	}
	if n := m.plan.Count(migrate.ActionUnsupported); n > 0 {
		details = append(details, fmt.Sprintf("%d unsupported", n))
	}
	if len(details) == 0 {
		return fmt.Sprintf("%d extensions in custom/", len(m.plan.Extensions))
	}
	return fmt.Sprintf("%d extensions in custom/ — %s", len(m.plan.Extensions), strings.Join(details, ", "))
}
