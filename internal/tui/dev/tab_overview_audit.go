package dev

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// ComposerAuditTimeout bounds `composer audit`, which queries the advisory
// database over the network, so it cannot hold up the dashboard or the start.
const ComposerAuditTimeout = 30 * time.Second

type composerAuditLoadedMsg struct {
	audit shop.ComposerAudit
	err   error
}

func loadComposerAudit(parent context.Context, exec executor.Executor) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, ComposerAuditTimeout)
		defer cancel()
		audit, err := RunComposerAudit(ctx, exec)
		return composerAuditLoadedMsg{audit: audit, err: err}
	}
}

// RunComposerAudit runs `composer audit` through the project's executor and
// returns the reported security advisories. Composer applies the project's
// audit configuration (ignored advisories, abandoned handling) itself.
func RunComposerAudit(ctx context.Context, exec executor.Executor) (shop.ComposerAudit, error) {
	if exec == nil {
		return shop.ComposerAudit{}, errors.New("cannot run composer audit: no executor")
	}

	// Composer exits non-zero when it finds advisories, so the output is
	// parsed first and the exit error only explains unparsable output.
	out, runErr := exec.ComposerCommand(ctx, "audit", "--format=json", "--no-interaction").Output()
	audit, err := shop.ParseComposerAudit(out)
	if err == nil {
		return audit, nil
	}
	if ctx.Err() != nil {
		return shop.ComposerAudit{}, errors.New("cannot run composer audit: timed out")
	}
	if runErr != nil {
		return shop.ComposerAudit{}, fmt.Errorf("cannot run composer audit: %s", commandFailureReason(runErr))
	}
	return shop.ComposerAudit{}, err
}

// commandFailureReason returns the last line the command wrote to stderr, or
// the error itself when there is none.
func commandFailureReason(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		lines := strings.Split(strings.TrimSpace(string(exitErr.Stderr)), "\n")
		if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
			return last
		}
	}
	return err.Error()
}

// ComposerAuditSummary describes the advisory count. It does not call the
// advisories vulnerabilities, since the Security Plugin may mitigate some.
func ComposerAuditSummary(audit shop.ComposerAudit) string {
	count := audit.Count()
	if count == 0 {
		return "no security advisories"
	}
	return fmt.Sprintf("%s in %s", plural(count, "security advisory", "security advisories"), plural(len(audit.Advisories), "package", "packages"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// renderComposerAuditRow is the Shop section's "Composer audit" row.
func (m OverviewModel) renderComposerAuditRow() string {
	var value string
	switch {
	case m.auditLoading:
		value = tui.DimStyle.Render("checking")
	case m.auditErr != nil:
		value = lipgloss.NewStyle().Foreground(tui.WarnColor).Render("unknown") + tui.DimStyle.Render(" ("+m.auditErr.Error()+")")
	case m.audit.Count() == 0:
		value = lipgloss.NewStyle().Foreground(tui.SuccessColor).Render(ComposerAuditSummary(m.audit))
	default:
		value = lipgloss.NewStyle().Foreground(tui.ErrorColor).Render(ComposerAuditSummary(m.audit)) + tui.DimStyle.Render(" (see Security advisories section below)")
	}
	return tui.KVRow("Composer audit", value)
}

// renderComposerAdvisories lists the affected packages with their advisories,
// each linked to the advisory where Composer reports a link.
func (m OverviewModel) renderComposerAdvisories() string {
	var s strings.Builder
	s.WriteString(tui.TitleStyle.Render("Security advisories"))
	s.WriteString("\n")
	for _, pkg := range m.audit.Packages() {
		s.WriteString("  ")
		s.WriteString(valueStyle.Render(pkg))
		s.WriteString("\n")
		for _, advisory := range m.audit.Advisories[pkg] {
			title := advisory.Title
			if advisory.CVE != "" {
				title = advisory.CVE + ": " + title
			}
			if advisory.Link != "" {
				title = tui.StyledLink(advisory.Link, title, tui.LinkStyle)
			}
			s.WriteString("    • ")
			s.WriteString(title)
			s.WriteString("\n")
		}
	}
	return s.String()
}
