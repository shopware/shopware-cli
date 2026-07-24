package upgradetui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// reviewState backs panel 4: the last read-only look at what will change.
type reviewState struct {
	button    int // 0 Start upgrade, 1 Export report
	exported  string
	exportErr error
}

func (m *Model) beginReview() (app.Content, tea.Cmd) {
	m.panel = panelReview
	m.review = reviewState{}
	return m, nil
}

// reportData assembles the shareable report from everything gathered so far.
func (m *Model) reportData() upgrade.ReportData {
	target := ""
	if t := m.check.target(); t != nil {
		target = t.Version.String()
	}
	current := ""
	if m.check.readiness.CurrentVersion != nil {
		current = m.check.readiness.CurrentVersion.String()
	}

	composerReport := ""
	var resolvedChanges []upgrade.PackageChange
	if m.prepare.resolve != nil {
		resolvedChanges = m.prepare.resolve.Changes
		if !m.prepare.resolve.OK {
			composerReport = m.prepare.resolve.Report
		}
	}

	return upgrade.ReportData{
		ProjectName:     projectName(m.opts.ProjectRoot),
		Current:         current,
		Target:          target,
		GeneratedAt:     time.Now(),
		Checks:          m.check.readiness.Checks,
		Extensions:      m.prepare.results,
		PlannedChanges:  m.plannedChanges(),
		PHPRequirement:  m.prepare.phpReq,
		PHPInstalled:    m.prepare.phpInstalled,
		ComposerReport:  composerReport,
		ResolvedChanges: resolvedChanges,
	}
}

// plannedChanges lists what the runner will do, for the review panel and report.
func (m *Model) plannedChanges() []string {
	return m.upgrader.PlannedChanges()
}

func (m *Model) updateReview(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case reportWrittenMsg:
		m.review.exported = msg.path
		m.review.exportErr = msg.err
		return m, nil

	case tea.KeyPressMsg:
		switch app.KeyString(msg) {
		case "up", "k", "left":
			if m.review.button > 0 {
				m.review.button--
			}
		case "down", "j", "right", "tab":
			if m.review.button < 1 {
				m.review.button++
			}
		case "esc":
			return m.backToPrepare()
		case "q":
			return m, tea.Quit
		case "enter":
			switch m.review.button {
			case 0:
				return m.beginRun()
			case 1:
				return m, exportReportCmd(m.upgrader, m.reportData())
			}
		}
	}
	return m, nil
}

func (m *Model) backToPrepare() (app.Content, tea.Cmd) {
	m.panel = panelPrepare
	return m, nil
}

func (m *Model) viewReview() (title, status, body string) {
	title = "Review upgrade plan and start"
	status = m.statusStrip(tui.VariantInfo, "TODO", "Review what will change before the local upgrade starts.")

	var left strings.Builder
	left.WriteString(tui.BoldStyle.Render("Planned project changes"))
	left.WriteString("\n\n")
	for _, change := range m.plannedChanges() {
		left.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render(change) + "\n")
	}
	left.WriteString("\n")

	left.WriteString(tui.BoldStyle.Render("Composer-managed extensions"))
	left.WriteString("\n\n")
	okCount, reviewCount, blockedCount := 0, 0, 0
	for _, r := range m.prepare.results {
		switch {
		case r.Status == upgrade.ExtOK:
			okCount++
		case r.Status.BlocksUpgrade():
			blockedCount++
		default:
			reviewCount++
		}
	}
	left.WriteString(fmt.Sprintf("  %-28s", fmt.Sprintf("%d compatible extensions", okCount)) + okStyle.Render("ok") + "\n")
	left.WriteString(fmt.Sprintf("  %-28s", fmt.Sprintf("%d extensions to review", reviewCount)) + okStyle.Render("reports ready") + "\n")
	left.WriteString(fmt.Sprintf("  %-28s", fmt.Sprintf("%d blocking extensions", blockedCount)) + okStyle.Render("ready") + "\n")
	left.WriteString("\n")

	left.WriteString(tui.BoldStyle.Render("Why these files change"))
	left.WriteString("\n")
	left.WriteString(tui.DimStyle.Render("Commit your changes after the wizard runs and tests"))
	left.WriteString("\n")
	left.WriteString(tui.DimStyle.Render("pass; composer.json and composer.lock make the"))
	left.WriteString("\n")
	left.WriteString(tui.DimStyle.Render("upgrade repeatable for deployment."))

	if m.review.exported != "" {
		left.WriteString("\n\n")
		left.WriteString(okStyle.Render("Report exported to " + m.review.exported))
	}
	if m.review.exportErr != nil {
		left.WriteString("\n\n")
		left.WriteString(failStyle.Render("Export failed: " + m.review.exportErr.Error()))
	}

	var right strings.Builder
	right.WriteString(tui.BoldStyle.Render("Deployment Helper workflow"))
	right.WriteString("\n\n")
	right.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("run Shopware update lifecycle") + "\n")
	right.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("update extension state where supported") + "\n")
	right.WriteString("\n\n")
	right.WriteString(userActionStyle.Render("User action"))
	right.WriteString("\n")
	right.WriteString(tui.LabelStyle.Render("Press ") + tui.BoldStyle.Render("Start upgrade") + tui.LabelStyle.Render(" to apply the plan"))
	right.WriteString("\n")
	right.WriteString(tui.LabelStyle.Render("locally, then run Composer and"))
	right.WriteString("\n")
	right.WriteString(tui.LabelStyle.Render("Deployment Helper."))
	right.WriteString("\n\n")
	right.WriteString(tui.BoldStyle.Render("After this step"))
	right.WriteString("\n")
	right.WriteString(tui.DimStyle.Render("The next screen shows progress and"))
	right.WriteString("\n")
	right.WriteString(tui.DimStyle.Render("logs while the upgrade is running."))
	right.WriteString("\n\n")
	right.WriteString(m.buttonWrap(m.rightColumnWidth(m.bodyWidth()*11/20),
		[]string{"Start upgrade", "Export report"}, m.review.button))

	body = m.twoColumn(m.bodyWidth()*11/20, left.String(), right.String())
	return title, status, body
}
