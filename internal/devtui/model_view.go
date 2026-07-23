package devtui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// windowTitle names the terminal window per phase, e.g. "[project] · Overview".
func (m Model) windowTitle() string {
	dir := "[" + filepath.Base(m.projectRoot) + "] · "

	switch m.phase {
	case phaseDashboard:
		return dir + tabNames[m.activeTab]
	case phaseStarting:
		return dir + "Starting..."
	case phaseStopping:
		return dir + "Stopping"
	case phaseInstallPrompt:
		return dir + "Install"
	case phaseInstalling:
		return dir + "Installing..."
	case phaseTask:
		return ""
	case phaseMigrationWizard:
		return dir + "Setup"
	}
	return dir + "shopware-cli"
}

// chromeHeader renders the shell header: the tab strip on the dashboard, the
// branding line everywhere else.
func (m Model) chromeHeader(ctx app.Context) string {
	if m.phase == phaseDashboard {
		return buildTabHeader(int(m.activeTab), ctx.Width)
	}
	return tui.BrandingHeader(ctx.Width)
}

// chromeFooter renders the shell footer: tab-specific shortcuts on the
// dashboard, the phase footer with its per-phase hint everywhere else.
func (m Model) chromeFooter(ctx app.Context) string {
	if m.phase == phaseDashboard {
		return m.renderDashboardFooter(ctx.Width)
	}
	return tui.PhaseFooter(m.phaseFooterHint(), "Exit")
}

func (m Model) phaseFooterHint() string {
	switch m.phase {
	case phaseStarting, phaseStopping, phaseInstalling:
		return tui.ShortcutBadge("l", "Toggle logs")
	case phaseInstallPrompt:
		return m.installFooterHint()
	case phaseMigrationWizard:
		return m.migrationWizard.footerHint()
	case phaseDashboard, phaseTask:
		return ""
	}
	return ""
}

// View renders the main region (the content box between the shell's header
// and footer chrome).
func (m Model) View(ctx app.Context) string {
	if ctx.Width == 0 || ctx.Height == 0 {
		return ""
	}

	switch m.phase {
	case phaseDashboard:
		return m.renderDashboard(ctx)
	case phaseStarting, phaseStopping, phaseInstallPrompt, phaseInstalling:
		return m.renderPhase(ctx)
	case phaseTask:
		return m.renderTask(ctx)
	case phaseMigrationWizard:
		return renderPhaseBox(m.migrationWizard.viewContent(), ctx.Width, ctx.MainHeight)
	}
	return ""
}

func (m Model) renderDashboard(ctx app.Context) string {
	boxHeight := ctx.MainHeight

	padV := 1
	padH := 3
	if m.activeTab == tabInstance {
		padV = 0
		padH = 1
	}

	contentH := boxHeight - padV*2 - 1
	contentW := ctx.Width - padH*2 - 2

	var content string
	switch m.activeTab {
	case tabOverview:
		content = m.overview.View(ctx.Width, boxHeight)
	case tabInstance:
		m.instance.SetSize(contentW, contentH)
		content = m.instance.View()
	case tabConfig:
		content = m.configTab.View(ctx.Width, boxHeight)
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(tui.BorderColor).
		Padding(padV, padH).
		Width(ctx.Width).
		Height(boxHeight)

	return contentBox.Render(content)
}

func (m Model) renderDashboardFooter(width int) string {
	switch m.activeTab {
	case tabInstance:
		return tui.ShortcutBarFit(width,
			tui.Shortcut{Key: "↑/↓", Label: "Navigate"},
			tui.Shortcut{Key: "enter", Label: "Select source"},
			tui.Shortcut{Key: "pgup/pgdn", Label: "Scroll logs"},
			tui.Shortcut{Key: "tab", Label: "Next tab"},
			tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
		)
	case tabConfig:
		return tui.ShortcutBarFit(width,
			tui.Shortcut{Key: "↑/↓", Label: "Navigate"},
			tui.Shortcut{Key: "enter", Label: "Edit/Save"},
			tui.Shortcut{Key: "tab", Label: "Next tab"},
			tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
		)
	case tabOverview:
		return tui.ShortcutBarFit(width,
			tui.Shortcut{Key: "↑/↓", Label: "Focus item"},
			tui.Shortcut{Key: "enter", Label: "Activate"},
			tui.Shortcut{Key: "ctrl+p", Label: "Commands"},
			tui.Shortcut{Key: "tab", Label: "Next tab"},
			tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
		)
	}

	return tui.ShortcutBarFit(width,
		tui.Shortcut{Key: "ctrl+p", Label: "Commands"},
		tui.Shortcut{Key: "tab", Label: "Next tab"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	)
}

func (m Model) renderPhase(ctx app.Context) string {
	var content strings.Builder

	switch m.phase {
	case phaseStarting:
		if m.dockerShowLogs {
			return m.renderDockerLogs("Starting Docker containers...", ctx.Width, ctx.MainHeight)
		}
		cardContent := fmt.Sprintf("%s Starting Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case phaseStopping:
		if m.dockerShowLogs {
			return m.renderDockerLogs("Stopping Docker containers...", ctx.Width, ctx.MainHeight)
		}
		cardContent := fmt.Sprintf("%s Stopping Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case phaseInstallPrompt:
		var card strings.Builder
		m.renderInstallPrompt(&card)
		content.WriteString(tui.RenderPhaseCard(card.String()))
	case phaseInstalling:
		if m.installProg.showLogs {
			return m.renderDockerLogs("Installing Shopware...", ctx.Width, ctx.MainHeight)
		}
		var card strings.Builder
		total := len(installStepPatterns)
		pctText := fmt.Sprintf(" %d%%", int(float64(m.installProg.currentStep)/float64(total)*100))
		card.WriteString(m.installProg.progress.View())
		card.WriteString(tui.DimStyle.Render(pctText))
		card.WriteString("\n\n")

		for i, sp := range installStepPatterns {
			switch {
			case i < m.installProg.currentStep:
				card.WriteString(tui.StepDone(sp.label))
			case i == m.installProg.currentStep && !m.installProg.done:
				card.WriteString(tui.StepActive(m.installProg.spinner.View(), sp.label))
			case i == m.installProg.currentStep && m.installProg.done:
				card.WriteString(tui.StepDone(sp.label))
			default:
				card.WriteString(tui.StepPending(tui.DimStyle.Render(sp.label)))
			}
		}
		content.WriteString(tui.RenderPhaseCard(strings.TrimRight(card.String(), "\n")))
	case phaseDashboard, phaseTask, phaseMigrationWizard:
		// Rendered by the outer View() dispatch, not here.
	}

	return renderPhaseBox(content.String(), ctx.Width, ctx.MainHeight)
}

// renderPhaseBox renders content centered inside the rounded main-region box.
func renderPhaseBox(content string, width, boxHeight int) string {
	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(width).
		Height(boxHeight).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center)

	contentWidth := lipgloss.Width(content)
	normalized := lipgloss.NewStyle().Width(contentWidth).Render(content)

	return contentBox.Render(normalized)
}

// renderTask renders the phaseTask log screen: the task's streamed output
// plus a dismissal hint once it finished.
func (m Model) renderTask(ctx app.Context) string {
	lines := m.task.Lines()
	if m.task.Done() {
		hint := helpStyle.Render("Done. Press any key to close.")
		if err := m.task.Err(); err != nil {
			hint = errorStyle.Render("Failed: " + err.Error())
		}
		lines = append(slices.Clone(lines), "", hint)
	}
	return m.renderLogScreen(m.task.StatusTitle(), lines, ctx.Width, ctx.MainHeight)
}

func (m Model) renderDockerLogs(title string, width, boxHeight int) string {
	return m.renderLogScreen(title, m.overlayLines, width, boxHeight)
}

func (m Model) renderLogScreen(title string, lines []string, width, boxHeight int) string {
	visibleLines := boxHeight - 6
	if visibleLines < 1 {
		visibleLines = 1
	}

	var body strings.Builder
	body.WriteString(panelHeaderStyle.Render(title))
	body.WriteString("\n\n")

	for _, line := range tui.TailLines(lines, visibleLines) {
		body.WriteString(line)
		body.WriteString("\n")
	}
	if len(lines) == 0 {
		body.WriteString(helpStyle.Render("Waiting for command output..."))
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(width).
		Height(boxHeight)

	return contentBox.Render(body.String())
}

func (m Model) overlayMaxLines() int {
	if m.height <= 0 {
		return 10
	}
	const overhead = 6
	maxLines := m.height - 2 - overhead
	if maxLines < 10 {
		return 10
	}
	return maxLines
}
