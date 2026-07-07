package projectupgrade

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

// upgradeDocsURL is the doc hint shown in the wizard header.
const upgradeDocsURL = "https://developer.shopware.com/docs/guides/installation/requirements.html"

func (m wizardModel) View() tea.View {
	content := lipgloss.JoinVertical(lipgloss.Left, m.headerBar(), "", m.viewContent())
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	return v
}

// windowTitle mirrors devtui's convention of keeping the terminal window
// title in sync with the current step or view.
func (m wizardModel) windowTitle() string {
	dir := "[" + filepath.Base(m.opts.ProjectRoot) + "] · "
	switch m.phase {
	case phaseWelcome:
		return dir + "Upgrade"
	case phasePreflight:
		return dir + "Preflight checks"
	case phaseSelectVersion:
		return dir + "Select target version"
	case phasePrepare:
		return dir + "Prepare upgrade"
	case phaseReview:
		return dir + "Review upgrade plan"
	case phaseRunning:
		return dir + "Upgrading..."
	case phaseDone:
		if m.finalErr != nil {
			return dir + "Upgrade failed"
		}
		return dir + "Upgrade complete"
	}
	return dir + "Upgrade"
}

// headerBar renders the consistent wizard header: current project context on
// the left, the environment indicator in the center, and the doc/source hint
// plus the selected target version on the right.
func (m wizardModel) headerBar() string {
	w := m.headerWidth()

	left := tui.BoldText.Render("Shopware " + m.opts.CurrentVersion.String())

	env := "local"
	if m.opts.Executor != nil {
		env = m.opts.Executor.Type()
	}
	center := tui.TextBadge(env)

	right := tui.StyledLink(upgradeDocsURL, "Docs", tui.LinkStyle)
	if m.targetVersion != "" {
		right = lipgloss.NewStyle().Foreground(tui.SuccessColor).Bold(true).Render("→ "+m.targetVersion) + tui.DimStyle.Render(" · ") + right
	}

	third := w / 3
	leftCell := lipgloss.NewStyle().Width(third).Render(left)
	centerCell := lipgloss.PlaceHorizontal(w-2*third, lipgloss.Center, center)
	rightCell := lipgloss.PlaceHorizontal(third, lipgloss.Right, right)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCell, centerCell, rightCell)
}

// headerWidth is the width the header aligns to: the phase card, or the full
// terminal in the running phase where the log spans the whole screen.
func (m wizardModel) headerWidth() int {
	if m.phase == phaseRunning && m.width > tui.PhaseCardWidth {
		return m.width
	}
	return tui.PhaseCardWidth
}

func (m wizardModel) viewContent() string {
	switch m.phase {
	case phaseWelcome:
		return m.viewWelcome()
	case phasePreflight:
		return m.viewPreflight()
	case phaseSelectVersion:
		return m.viewSelectVersion()
	case phasePrepare:
		return m.viewPrepare()
	case phaseReview:
		return m.viewReview()
	case phaseRunning:
		return m.viewRunning()
	case phaseDone:
		return m.viewDone()
	}
	return ""
}

func (m wizardModel) viewWelcome() string {
	var b strings.Builder
	b.WriteString(tui.TextBadge("Upgrade"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Upgrade Shopware to a newer version"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("The wizard makes upgrade risk visible before downtime:"))
	b.WriteString("\n\n")
	for _, line := range []string{
		"Check that this project can safely attempt the upgrade",
		"Show which extensions are compatible, need review, or block the upgrade",
		"Let composer resolve the target version and update your project",
		"Run vendor/bin/shopware-deployment-helper for migrations and theme compile",
		"Export a Markdown report you can share with support or extension vendors",
	} {
		b.WriteString(tui.DimStyle.Render("  • "))
		b.WriteString(tui.LabelStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("It will not make incompatible extensions compatible, rewrite"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("custom plugin code, resolve missing vendor releases, or guarantee"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("a production upgrade succeeds without testing."))
	b.WriteString("\n")
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 8))
	b.WriteString(tui.KVRow("Current version", tui.BoldText.Render(m.opts.CurrentVersion.String())))
	b.WriteString(tui.KVRow("Project root", tui.DimStyle.Render(m.opts.ProjectRoot)))
	b.WriteString("\n")
	b.WriteString(renderConfirmButtons("Begin upgrade", "Cancel", m.confirmYes))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCardCowsay("Let's get this Shopware up to date!", b.String())
}

func (m wizardModel) viewPreflight() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Preflight checks"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The project must pass these checks before the upgrade flow starts."))
	b.WriteString("\n\n")

	if m.preflightLoading {
		b.WriteString(m.spinner.View() + " " + tui.DimStyle.Render("Running checks…"))
		b.WriteString("\n\n")
		b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Exit"}))
		return tui.RenderPhaseCard(b.String())
	}

	for _, r := range m.preflight {
		b.WriteString(renderPreflightLine(r))
	}

	if PreflightBlocked(m.preflight) {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("Fix the failing checks, then recheck."))
		b.WriteString("\n")
		for _, r := range m.preflight {
			if r.Status != PreflightFailed || r.Explanation == "" {
				continue
			}
			b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render("  ✗ " + r.Label + ": "))
			b.WriteString(tui.DimStyle.Render(r.Explanation))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(m.footer(
			tui.Shortcut{Key: "r", Label: "Recheck"},
			tui.Shortcut{Key: "q", Label: "Exit"},
		))
	} else {
		b.WriteString("\n")
		b.WriteString(tui.GreenText.Render("All checks passed."))
		b.WriteString("\n\n")
		b.WriteString(m.footer(
			tui.Shortcut{Key: "enter", Label: "Continue"},
			tui.Shortcut{Key: "r", Label: "Recheck"},
			tui.Shortcut{Key: "q", Label: "Exit"},
		))
	}

	return tui.RenderPhaseCard(b.String())
}

func renderPreflightLine(r PreflightResult) string {
	var icon string
	switch r.Status {
	case PreflightOK:
		icon = tui.Checkmark
	case PreflightFailed:
		icon = lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗")
	case PreflightSkipped:
		icon = tui.DimStyle.Render("·")
	default:
		icon = tui.DimStyle.Render("○")
	}

	line := "  " + icon + "  " + tui.LabelStyle.Render(r.Label)
	if r.Detail != "" {
		line += " " + tui.DimStyle.Render("("+r.Detail+")")
	}
	return line + "\n"
}

func (m wizardModel) viewSelectVersion() string {
	var b strings.Builder
	b.WriteString(m.versionList.View())
	b.WriteString("\n\n")

	if selected, ok := m.versionList.Selected(); ok {
		b.WriteString(tui.DimStyle.Render("Release notes: "))
		b.WriteString(tui.StyledLink(ReleaseNotesURL(selected.Label), ReleaseNotesURL(selected.Label), tui.LinkStyle))
		b.WriteString("\n\n")
	}

	shortcuts := append(m.versionList.Shortcuts(),
		tui.Shortcut{Key: "enter", Label: "Continue"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	)
	b.WriteString(m.footer(shortcuts...))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewPrepare() string {
	if m.compatLoading {
		return m.viewPrepareLoading()
	}
	if m.overlayOpen {
		return m.viewExtensionOverlay()
	}

	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Prepare upgrade"))
	b.WriteString("\n\n")

	// Overall preparation state.
	blockers := CountBlockers(m.extQueue)
	switch {
	case blockers > 0:
		b.WriteString(tui.StatusBadge("BLOCKED", tui.ErrorColor))
		b.WriteString("  ")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render(fmt.Sprintf("%d/%d extensions need fixes before the upgrade can continue.", blockers, len(m.extQueue))))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("  TODO: Update or remove blocked extensions, then recheck (r)."))
		b.WriteString("\n\n")
	case m.compatErr != nil:
		b.WriteString(tui.StatusBadge("REVIEW", tui.WarnColor))
		b.WriteString("  ")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.WarnColor).Render("Compatibility check failed: " + m.compatErr.Error()))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("  You may still continue; the wizard cannot guarantee extensions will install."))
		b.WriteString("\n\n")
	default:
		b.WriteString(tui.StatusBadge("READY", tui.SuccessColor))
		b.WriteString("  ")
		b.WriteString(tui.GreenText.Render("All extensions allow the upgrade to continue."))
		b.WriteString("\n\n")
	}

	// System preparation checks.
	b.WriteString(renderSystemCheck(m.preflightStatusByLabel("Web environment running"), "Web services running"))
	b.WriteString(renderSystemCheck(m.preflightStatusByLabel("Packagist reachable"), "Packagist reachable"))
	b.WriteString(renderSystemCheck(m.compatErr == nil && m.compatReport.OK, "Composer can resolve this upgrade"))
	b.WriteString(renderSystemCheck(blockers == 0, "Extension compatibility"))

	if len(m.extQueue) > 0 {
		b.WriteString("\n")
		b.WriteString(m.renderExtensionTable())
	} else {
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("No extensions installed."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	shortcuts := []tui.Shortcut{
		{Key: "↑/↓", Label: "Rows"},
		{Key: "enter", Label: "Details"},
		{Key: "r", Label: "Recheck"},
	}
	if blockers == 0 {
		shortcuts = append(shortcuts, tui.Shortcut{Key: "c", Label: "Continue"})
	}
	shortcuts = append(shortcuts, tui.Shortcut{Key: "q", Label: "Exit"})
	b.WriteString(m.footer(shortcuts...))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewPrepareLoading() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Prepare upgrade"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Asking composer to resolve %s…", m.targetVersion)))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " " + tui.DimStyle.Render("composer require --dry-run"))
	b.WriteString("\n\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Cancel"}))
	return tui.RenderPhaseCard(b.String())
}

// preflightStatusByLabel reports whether the named preflight check passed
// (or was skipped/absent, which does not block preparation).
func (m wizardModel) preflightStatusByLabel(label string) bool {
	for _, r := range m.preflight {
		if r.Label == label {
			return r.Status != PreflightFailed
		}
	}
	return true
}

func renderSystemCheck(ok bool, label string) string {
	icon := tui.Checkmark
	if !ok {
		icon = lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗")
	}
	return "  " + icon + "  " + tui.LabelStyle.Render(label) + "\n"
}

// extensionStateBadge renders the semantic state cell used in the extension
// queue and detail overlays.
func extensionStateBadge(state ExtensionState) string {
	switch state {
	case ExtensionBlocked:
		return lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("BLOCKED")
	case ExtensionDeprecated:
		return lipgloss.NewStyle().Foreground(tui.WarnColor).Bold(true).Render("REPLACE")
	case ExtensionUpdate:
		return lipgloss.NewStyle().Foreground(tui.WarnColor).Render("UPDATE")
	case ExtensionRemove:
		return tui.DimStyle.Render("REMOVE")
	case ExtensionOK:
		return lipgloss.NewStyle().Foreground(tui.SuccessColor).Render("OK")
	}
	return ""
}

func (m wizardModel) renderExtensionTable() string {
	var b strings.Builder

	// The card interior is PhaseCardWidth minus borders (2) and padding (6);
	// each row additionally spends 2 columns on the cursor.
	innerW := tui.PhaseCardWidth - 10
	stateW, nameW, versionW := 9, 24, 19
	resultW := innerW - stateW - nameW - versionW

	header := lipgloss.NewStyle().Bold(true).Render(
		padCell("State", stateW) + padCell("Name", nameW) + padCell("Current → Target", versionW) + padCell("Result", resultW),
	)
	b.WriteString("  " + header + "\n")
	b.WriteString("  " + tui.TableDivider(innerW) + "\n")

	start, end := 0, len(m.extQueue)
	windowed := len(m.extQueue) > maxVisibleExtensions
	if windowed {
		start = m.extCursor - maxVisibleExtensions/2
		if start < 0 {
			start = 0
		}
		if start > len(m.extQueue)-maxVisibleExtensions {
			start = len(m.extQueue) - maxVisibleExtensions
		}
		end = start + maxVisibleExtensions
	}

	for i := start; i < end; i++ {
		row := m.extQueue[i]
		versions := row.Current
		if row.Target != "" && row.Target != row.Current {
			versions += " → " + row.Target
		}
		if row.Target == "" {
			versions += " → —"
		}

		cursor := "  "
		if i == m.extCursor {
			cursor = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("● ")
		}

		nameStyle := tui.LabelStyle
		if i == m.extCursor {
			nameStyle = nameStyle.Bold(true)
		}

		b.WriteString(cursor)
		b.WriteString(padCell(extensionStateBadge(row.State), stateW))
		b.WriteString(nameStyle.Render(padCellPlain(row.Name, nameW)))
		b.WriteString(tui.DimStyle.Render(padCellPlain(versions, versionW)))
		b.WriteString(tui.DimStyle.Render(padCellPlain(row.Result, resultW)))
		b.WriteString("\n")
	}

	if windowed {
		b.WriteString("  " + tui.DimStyle.Render(fmt.Sprintf("Showing %d–%d of %d (↑/↓ to scroll)", start+1, end, len(m.extQueue))))
		b.WriteString("\n")
	}

	return b.String()
}

// padCell pads styled content to a fixed visual width.
func padCell(content string, width int) string {
	gap := width - lipgloss.Width(content)
	if gap < 1 {
		gap = 1
	}
	return content + strings.Repeat(" ", gap)
}

// padCellPlain truncates and pads unstyled text to a fixed width.
func padCellPlain(content string, width int) string {
	return padCell(tui.TruncateToWidth(content, width-1), width)
}

func (m wizardModel) viewExtensionOverlay() string {
	if m.extCursor >= len(m.extQueue) {
		return ""
	}
	row := m.extQueue[m.extCursor]

	var b strings.Builder
	b.WriteString(tui.TextBadge("Extension details"))
	b.WriteString("  ")
	b.WriteString(extensionStateBadge(row.State))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render(row.Name))
	b.WriteString("\n\n")

	b.WriteString(tui.KVRow("Installed", tui.BoldText.Render(row.Current)))
	target := row.Target
	if target == "" {
		target = "no compatible release"
	}
	b.WriteString(tui.KVRow("Target", tui.LabelStyle.Render(target)))
	b.WriteString(tui.KVRow("Result", tui.LabelStyle.Render(row.Result)))
	if row.Replacement != "" {
		b.WriteString(tui.KVRow("Replacement", tui.YellowText.Render(row.Replacement)))
	}
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 8))

	b.WriteString(tui.BlueText.Bold(true).Render("User action"))
	b.WriteString("\n")
	for _, line := range extensionGuidance(row) {
		b.WriteString(tui.DimStyle.Render("  " + line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	shortcuts := []tui.Shortcut{{Key: "esc", Label: "Close"}, {Key: "r", Label: "Recheck"}}
	if row.State == ExtensionBlocked {
		shortcuts = append(shortcuts, tui.Shortcut{Key: "d", Label: "Remove during upgrade"})
	}
	if row.State == ExtensionRemove {
		shortcuts = append(shortcuts, tui.Shortcut{Key: "d", Label: "Keep (undo remove)"})
	}
	b.WriteString(m.footer(shortcuts...))
	return tui.RenderPhaseCard(b.String())
}

// extensionGuidance is the contextual help shown in the detail overlay,
// mirroring the issue's per-state detail panels.
func extensionGuidance(row ExtensionRow) []string {
	switch row.State {
	case ExtensionOK:
		return []string{"No action required. The installed release is compatible with the target version."}
	case ExtensionUpdate:
		return []string{
			"Composer will update this extension during the upgrade.",
			"Review the extension changelog before continuing, then test the affected features after the upgrade.",
		}
	case ExtensionDeprecated:
		guidance := []string{"This extension is abandoned by its vendor and will not receive further updates."}
		if row.Replacement != "" {
			guidance = append(guidance, "Replace it with "+row.Replacement+" instead of updating it.")
		} else {
			guidance = append(guidance, "No replacement was suggested. Plan to remove or replace it.")
		}
		return guidance
	case ExtensionBlocked:
		return []string{
			"No release of this extension is compatible with the target version.",
			"Update the extension once the vendor publishes a compatible release, then recheck.",
			"Or press d to remove it from composer.json during the upgrade — its features will be gone until you re-require it.",
		}
	case ExtensionRemove:
		return []string{
			"You chose to remove this extension from composer.json during the upgrade.",
			"Re-require it once the vendor publishes a compatible release. Press d to keep it instead.",
		}
	}
	return nil
}

func (m wizardModel) viewReview() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Review upgrade plan"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Confirm to apply the following changes."))
	b.WriteString("\n\n")
	b.WriteString(tui.KVRow("From", tui.BoldText.Render(m.opts.CurrentVersion.String())))
	b.WriteString(tui.KVRow("To", lipgloss.NewStyle().Foreground(tui.SuccessColor).Bold(true).Render(m.targetVersion)))
	if m.opts.Executor != nil {
		b.WriteString(tui.KVRow("Executor", tui.LabelStyle.Render(m.opts.Executor.Type())))
	}
	if m.phpRequirement != "" {
		b.WriteString(tui.KVRow("PHP requirement", tui.LabelStyle.Render(m.phpRequirement)))
	}
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 8))
	b.WriteString(tui.DimStyle.Render("Tasks to be executed:"))
	b.WriteString("\n")
	for _, t := range m.tasks {
		b.WriteString(tui.DimStyle.Render("  • "))
		b.WriteString(tui.LabelStyle.Render(t.label))
		b.WriteString("\n")
	}

	if removals := m.plannedRemovals(); len(removals) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.WarnColor).Render("These extensions will be removed from composer.json:"))
		b.WriteString("\n")
		for _, row := range removals {
			b.WriteString(tui.DimStyle.Render("  • "))
			b.WriteString(tui.LabelStyle.Render(row.Name))
			b.WriteString("\n")
		}
	}

	b.WriteString(m.reportStatusLine())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(tui.WarnColor).Render("⚠  Commit your changes before continuing."))
	b.WriteString("\n\n")
	b.WriteString(renderConfirmButtons("Start upgrade", "Cancel", m.confirmYes))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "e", Label: "Report"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCard(b.String())
}

// reportStatusLine renders the outcome of the last report export, if any.
func (m wizardModel) reportStatusLine() string {
	if m.reportErr != nil {
		return "\n" + lipgloss.NewStyle().Foreground(tui.ErrorColor).Render("Report export failed: "+m.reportErr.Error()) + "\n"
	}
	if m.reportPath != "" {
		return "\n" + tui.GreenText.Render("Report written to "+m.reportPath) + "\n"
	}
	return ""
}

func (m wizardModel) viewRunning() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render(fmt.Sprintf("Upgrading to %s", m.targetVersion)))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("This may take a few minutes. Live output shown below."))
	b.WriteString("\n\n")

	for i, t := range m.tasks {
		b.WriteString(m.renderTaskLine(i, t))
		b.WriteString("\n")
	}

	// The live log spans the full terminal width so long composer lines stay
	// readable, unlike the fixed-width card above it.
	logWidth := m.width
	if logWidth <= 0 {
		logWidth = tui.PhaseCardWidth
	}

	var log strings.Builder
	if len(m.logLines) > 0 {
		log.WriteString(lipgloss.NewStyle().Foreground(tui.BorderColor).Render(strings.Repeat("─", logWidth)))
		log.WriteString("\n")
		for _, line := range m.logLines {
			log.WriteString(tui.DimStyle.Render(truncate(line, logWidth)))
			log.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Cancel"}))

	card := tui.RenderPhaseCard(b.String())
	if log.Len() == 0 {
		return card
	}
	return lipgloss.JoinVertical(lipgloss.Left, card, "", strings.TrimRight(log.String(), "\n"))
}

// postUpgradeChecklist is shown after a successful upgrade so users know what
// to validate before going to production.
var postUpgradeChecklist = []string{
	"Open the storefront and click through key pages",
	"Log in to the administration",
	"Verify your theme renders correctly (recompile if needed)",
	"Run a test order through checkout and payment",
	"Exercise the critical features of your extensions",
	"Check var/log/ for new errors",
	"Run your test suite before deploying to production",
}

func (m wizardModel) viewDone() string {
	var b strings.Builder

	if m.finalErr != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor).Render("✗ Upgrade failed"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render(m.finalErr.Error()))
		b.WriteString("\n\n")

		if tail := lastLines(m.fullLog, maxLogLines); len(tail) > 0 {
			b.WriteString(tui.BoldText.Render("Last output:"))
			b.WriteString("\n")
			for _, line := range tail {
				b.WriteString(tui.DimStyle.Render("  " + truncate(line, tui.PhaseCardWidth-10)))
				b.WriteString("\n")
			}
			b.WriteString("\n")
			b.WriteString(tui.DimStyle.Render("The full log is printed below the wizard after you close it."))
			b.WriteString("\n\n")
		}

		b.WriteString(tui.DimStyle.Render("composer.json and vendor are left as-is; run `git checkout composer.json composer.lock` to revert."))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.SuccessColor).Render(fmt.Sprintf("✓ Upgraded to Shopware %s", m.targetVersion)))
		b.WriteString("\n\n")
		b.WriteString(tui.BoldText.Render("Post-upgrade validation checklist:"))
		b.WriteString("\n")
		for _, item := range postUpgradeChecklist {
			b.WriteString(tui.DimStyle.Render("  ☐ "))
			b.WriteString(tui.LabelStyle.Render(item))
			b.WriteString("\n")
		}

		if m.pluginActions != nil {
			removed := m.pluginActions.Removed()

			if len(removed) > 0 {
				b.WriteString("\n")
				b.WriteString(tui.BoldText.Render("Removed incompatible custom plugins:"))
				b.WriteString("\n")
				for _, action := range removed {
					b.WriteString(tui.DimStyle.Render("  • "))
					b.WriteString(tui.LabelStyle.Render(action.Name))
					if action.Reason != "" {
						b.WriteString(tui.DimStyle.Render(" (" + action.Reason + ")"))
					}
					b.WriteString("\n")
				}
				b.WriteString(tui.DimStyle.Render("Re-require them in composer.json once they publish compatible versions."))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString(m.reportStatusLine())
	b.WriteString("\n")
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 8))
	for i, t := range m.tasks {
		b.WriteString(m.renderTaskLine(i, t))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "e", Label: "Export report"},
		tui.Shortcut{Key: "enter", Label: "Close"},
	))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) renderTaskLine(i int, t task) string {
	var icon string
	switch t.status {
	case taskRunning:
		icon = m.spinner.View()
	case taskDone:
		icon = tui.Checkmark
	case taskFailed:
		icon = lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗")
	case taskSkipped:
		icon = tui.DimStyle.Render("·")
	case taskPending:
		icon = tui.DimStyle.Render("○")
	default:
		icon = tui.DimStyle.Render("○")
	}

	style := tui.LabelStyle
	if t.status == taskPending {
		style = tui.DimStyle
	}

	line := fmt.Sprintf("  %s  %s", icon, style.Render(t.label))
	if t.detail != "" {
		line += " " + tui.DimStyle.Render("("+t.detail+")")
	}
	if i == m.currentTask && t.status == taskRunning {
		line = lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}

func (m wizardModel) footer(shortcuts ...tui.Shortcut) string {
	return tui.ShortcutBar(shortcuts...)
}

func renderConfirmButtons(yesLabel, noLabel string, yesActive bool) string {
	yesStyle := lipgloss.NewStyle().Foreground(tui.TextColor).Background(tui.BrandColor).Padding(0, 2)
	noStyle := lipgloss.NewStyle().Foreground(tui.MutedColor).Background(tui.SubtleBgColor).Padding(0, 2)

	var yes, no string
	if yesActive {
		yes = yesStyle.Render(yesLabel)
		no = noStyle.Render(noLabel)
	} else {
		yes = noStyle.Render(yesLabel)
		no = yesStyle.Render(noLabel)
	}
	return yes + "  " + no
}

func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	if len([]rune(s)) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes-1]) + "…"
}

// lastLines returns up to n trailing lines of lines.
func lastLines(lines []string, n int) []string {
	if n <= 0 || len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
