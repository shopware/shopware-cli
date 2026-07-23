package upgradetui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// doneState backs panel 6: the summary after the upgrade finished or failed.
type doneState struct {
	succeeded bool
	err       error
}

func (m *Model) beginDone() (app.Content, tea.Cmd) {
	m.panel = panelDone
	m.done = doneState{
		succeeded: m.run.succeeded,
		err:       m.run.err,
	}
	return m, m.trackUpgradeCmd()
}

// trackUpgradeCmd reports the upgrade outcome to telemetry.
func (m *Model) trackUpgradeCmd() tea.Cmd {
	result := tracking.ResultFailure
	if m.done.succeeded {
		result = tracking.ResultSuccess
	}

	current := ""
	if m.check.readiness.CurrentVersion != nil {
		current = m.check.readiness.CurrentVersion.String()
	}
	target := ""
	if t := m.check.target(); t != nil {
		target = t.Version.String()
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		tracking.Track(ctx, tracking.EventProjectUpgrade, map[string]string{
			tracking.TagFromVersion:   current,
			tracking.TagTargetVersion: target,
			tracking.TagResult:        result,
		})
		return nil
	}
}

func (m *Model) updateDone(msg tea.Msg) (app.Content, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch app.KeyString(key) {
	case "enter", "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewDone() (title, status, body string) {
	if m.done.succeeded {
		title = "Upgrade report"
		status = m.statusStrip(tui.VariantSuccess, "DONE", "All tasks completed. Verify your shop and run your test suite.")
	} else {
		title = "Upgrade failed"
		message := "The upgrade was rolled back."
		if m.done.err != nil {
			message = tui.Truncate(m.done.err.Error(), m.bodyWidth()-16)
		}
		status = m.statusStrip(tui.VariantError, "FAILED", message)
	}
	body = m.twoColumn(m.bodyWidth()*11/20, m.viewDoneLeft(), m.viewDoneRight())
	return title, status, body
}

func (m *Model) viewDoneLeft() string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("What happened"))
	b.WriteString("\n\n")

	if m.done.succeeded {
		current := ""
		if m.check.readiness.CurrentVersion != nil {
			current = m.check.readiness.CurrentVersion.String()
		}
		b.WriteString(stateDot(upgrade.StateOK) + " " + tui.LabelStyle.Render("Shopware packages updated"))
		b.WriteString("\n")
		if t := m.check.target(); t != nil {
			b.WriteString(tui.DimStyle.Render("   "+current+" -> "+t.Version.String()) + "\n")
		}
		for _, line := range []string{
			"composer.json updated",
			"composer.lock updated",
			"Deployment Helper completed",
			"Composer-managed extensions checked",
		} {
			b.WriteString(stateDot(upgrade.StateOK) + " " + tui.LabelStyle.Render(line) + "\n")
		}
	} else {
		b.WriteString(stateDot(upgrade.StateFail) + " " + tui.LabelStyle.Render("The upgrade did not complete"))
		b.WriteString("\n")
		b.WriteString(stateDot(upgrade.StateOK) + " " + tui.LabelStyle.Render("composer.json and composer.lock were restored"))
		b.WriteString("\n")
		if m.done.err != nil {
			b.WriteString("\n")
			b.WriteString(failStyle.Render(m.done.err.Error()))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Reports written"))
	b.WriteString("\n\n")
	// The runner writes a report on success and failure alike, but a run that
	// died before its first step (log/backup failure) leaves none — only link
	// what actually exists.
	reportPath := m.upgrader.ReportPath()
	if _, err := os.Stat(reportPath); err == nil {
		b.WriteString(tui.DimStyle.Render("  • ") + tui.StyledLink("file://"+reportPath, relativePath(m.opts.ProjectRoot, reportPath), tui.LinkStyle) + "\n")
	}
	logPath := m.upgrader.LogPath()
	if _, err := os.Stat(logPath); err == nil {
		b.WriteString(tui.DimStyle.Render("  • ") + tui.StyledLink("file://"+logPath, relativePath(m.opts.ProjectRoot, logPath), tui.LinkStyle) + "\n")
	}

	return b.String()
}

func (m *Model) viewDoneRight() string {
	var b strings.Builder
	b.WriteString(userActionStyle.Render("User action"))
	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Next steps"))
	b.WriteString("\n\n")

	var steps []string
	if m.done.succeeded {
		steps = []string{
			"Review the upgrade report",
			"Double-check the shop locally",
			"Run storefront and admin smoke tests",
			"Review local extension reports",
			"Commit composer.json and composer.lock",
			"Deploy through your normal process",
		}
	} else {
		steps = []string{
			"Read the log to find the failing step",
			"Run composer install to restore vendor/",
			"Fix the reported problem",
			"Start the wizard again",
		}
	}
	for i, step := range steps {
		b.WriteString(tui.DimStyle.Render("  "+string(rune('1'+i))+". ") + tui.LabelStyle.Render(step) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Local-first handoff"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The wizard upgraded this project"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("locally. Test and commit before deploying."))
	b.WriteString("\n\n")
	b.WriteString(m.buttonRow([]string{"Close"}, 0))

	return b.String()
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
