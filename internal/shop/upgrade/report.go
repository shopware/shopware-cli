package upgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpgradeDir returns the directory below the project root where the wizard
// stores its report, log, and backups.
func (u *ProjectUpgrader) UpgradeDir() string {
	return filepath.Join(u.projectRoot, ".shopware-cli", "upgrade")
}

// ReportPath returns the location of the Markdown upgrade report.
func (u *ProjectUpgrader) ReportPath() string {
	return filepath.Join(u.UpgradeDir(), "report.md")
}

// ReportData collects everything that goes into the shareable upgrade report.
type ReportData struct {
	ProjectName    string
	Current        string
	Target         string
	GeneratedAt    time.Time
	Checks         []ReadinessCheck
	Extensions     []ExtensionResult
	PlannedChanges []string
	// PHPRequirement is the PHP constraint of the target shopware/core
	// release; PHPInstalled the locally detected version.
	PHPRequirement string
	PHPInstalled   string
	// ComposerReport is the raw Composer output attached when dependency
	// resolution failed.
	ComposerReport string
	// ResolvedChanges are the lock-file operations the dry run predicted.
	ResolvedChanges []PackageChange
	// Failed marks a report written for an upgrade that was rolled back;
	// Error carries the failing step's message.
	Failed bool
	Error  string
}

// WriteReport renders the Markdown upgrade report into the upgrade directory
// and returns its path.
func (u *ProjectUpgrader) WriteReport(data ReportData) (string, error) {
	dir := u.UpgradeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte(renderReport(data)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func renderReport(data ReportData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Shopware upgrade report — %s\n\n", data.ProjectName)
	fmt.Fprintf(&b, "- **From:** Shopware %s\n", data.Current)
	fmt.Fprintf(&b, "- **To:** Shopware %s\n", data.Target)
	if !data.GeneratedAt.IsZero() {
		fmt.Fprintf(&b, "- **Generated:** %s by shopware-cli\n", data.GeneratedAt.Format(time.RFC3339))
	}
	b.WriteString("\n")

	if data.Failed {
		b.WriteString("## Result: failed and rolled back\n\n")
		b.WriteString("The upgrade did not complete; composer.json and composer.lock were restored.\n\n")
		if data.Error != "" {
			fmt.Fprintf(&b, "```\n%s\n```\n\n", data.Error)
		}
	}

	b.WriteString("## System requirements\n\n")
	b.WriteString("| Requirement | Status |\n|---|---|\n")
	if data.PHPRequirement != "" {
		phpStatus := "unknown"
		if data.PHPInstalled != "" {
			phpStatus = "installed: " + data.PHPInstalled
		}
		fmt.Fprintf(&b, "| PHP %s | %s |\n", data.PHPRequirement, phpStatus)
	}
	for _, c := range data.Checks {
		fmt.Fprintf(&b, "| %s | %s %s |\n", c.Label, stateMarker(c.State), c.Value)
	}
	b.WriteString("\n")

	if len(data.PlannedChanges) > 0 {
		b.WriteString("## Planned Composer changes\n\n")
		for _, change := range data.PlannedChanges {
			fmt.Fprintf(&b, "- `%s`\n", change)
		}
		b.WriteString("\n")
	}

	writeExtensionGroup(&b, "Blocked", data.Extensions, func(s ExtStatus) bool {
		return s.BlocksUpgrade()
	})
	writeExtensionGroup(&b, "Needs review", data.Extensions, func(s ExtStatus) bool {
		return !s.BlocksUpgrade() && s != ExtOK
	})
	writeExtensionGroup(&b, "OK", data.Extensions, func(s ExtStatus) bool {
		return s == ExtOK
	})

	if len(data.ResolvedChanges) > 0 {
		b.WriteString("## Resolved package changes\n\n")
		b.WriteString("| Package | From | To | Operation |\n|---|---|---|---|\n")
		for _, change := range data.ResolvedChanges {
			from, to := change.From, change.To
			if from == "" {
				from = "—"
			}
			if to == "" {
				to = "—"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", change.Name, from, to, change.Op)
		}
		b.WriteString("\n")
	}

	if data.ComposerReport != "" {
		b.WriteString("## Composer resolution report\n\n")
		b.WriteString("```\n")
		b.WriteString(strings.TrimRight(data.ComposerReport, "\n"))
		b.WriteString("\n```\n\n")
	}

	b.WriteString("---\n\nThe wizard applies changes locally. Test the shop, commit composer.json and composer.lock, and deploy through your normal process.\n")

	return b.String()
}

func writeExtensionGroup(b *strings.Builder, title string, extensions []ExtensionResult, match func(ExtStatus) bool) {
	var rows []ExtensionResult
	for _, e := range extensions {
		if match(e.Status) {
			rows = append(rows, e)
		}
	}
	if len(rows) == 0 {
		return
	}

	fmt.Fprintf(b, "## Extensions: %s (%d)\n\n", title, len(rows))
	b.WriteString("| Extension | Package | Installed | Compatible release | Result | Note |\n|---|---|---|---|---|---|\n")
	for _, e := range rows {
		available := e.Available
		if available == "" {
			available = "—"
		}
		pkg := e.Extension.Package
		if pkg == "" {
			pkg = "—"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n",
			e.Extension.Name, pkg, e.Extension.Version, available, e.Status.Label(), e.Detail)
	}
	b.WriteString("\n")
}

func stateMarker(s CheckState) string {
	switch s {
	case StateOK:
		return "✅"
	case StateWarn:
		return "⚠️"
	case StateFail:
		return "❌"
	case StatePending, StateRunning:
		return "⏳"
	}
	return ""
}
