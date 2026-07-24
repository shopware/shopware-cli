package upgrade

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
)

// Target keywords accepted by HeadlessOptions.Target besides an exact version.
const (
	TargetRecommended = "recommended"
	TargetLatestPatch = "latest-patch"
)

// HeadlessOptions configure a non-interactive upgrade run.
type HeadlessOptions struct {
	// Target is the version to upgrade to: an exact version from the
	// supported catalog, or the keywords "recommended" / "latest-patch".
	Target string
	// DryRun stops after the read-only preflight (readiness, extension
	// compatibility, Composer dry-run) without modifying any file.
	DryRun bool
	// NoAudit continues when dependencies are blocked by security advisories
	// (config.audit.block-insecure = false, like project create's --no-audit).
	NoAudit bool
	// Out receives the progress output.
	Out io.Writer
}

// RunHeadless executes the upgrade pipeline without the TUI — for
// --no-interaction runs and environments without a terminal. It walks the
// same phases as the wizard: readiness checks, target selection, preflight,
// and (unless DryRun) the guided execution with rollback on failure.
func (u *ProjectUpgrader) RunHeadless(ctx context.Context, opts HeadlessOptions) error {
	out := opts.Out
	if opts.NoAudit {
		u.DisableAuditBlock()
	}

	_, _ = fmt.Fprintln(out, tui.SectionHeadingStyle.Render("Readiness checks"))
	readiness := u.RunReadinessChecks(ctx)
	for _, check := range readiness.Checks {
		_, _ = fmt.Fprintf(out, "%s %s: %s\n", stateGlyph(check.State), check.Label, check.Value)
		if check.Detail != "" && check.State != StateOK {
			for line := range strings.SplitSeq(check.Detail, "\n") {
				_, _ = fmt.Fprintln(out, "  "+tui.DimText.Render(line))
			}
		}
	}
	if readiness.Blocked() {
		return fmt.Errorf("the project is not ready to upgrade; fix the failing checks above")
	}

	catalog, err := u.LoadCatalog(ctx, readiness.CurrentVersion)
	if err != nil {
		return fmt.Errorf("load available versions: %w", err)
	}
	if len(catalog.Options) == 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, tui.SuccessLine("Already on the latest supported Shopware version — nothing to do."))
		return nil
	}

	target, err := selectTarget(catalog, opts.Target)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "%s\n", tui.SectionHeadingStyle.Render(
		fmt.Sprintf("Upgrade Shopware %s -> %s", catalog.Current, target.Version)))

	results := u.CheckExtensions(ctx, readiness.CurrentVersion, target.Version, readiness.Extensions)
	resolve, err := u.resolveHeadless(ctx, target.Version.String())
	if err != nil {
		return err
	}
	ApplyResolvedVersions(results, resolve)
	printExtensionResults(out, results)

	report := u.headlessReportData(ctx, readiness, catalog, target, results, resolve)

	if !resolve.OK {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, tui.FailLine("Composer cannot resolve this upgrade:"))
		for _, line := range tui.TailLines(strings.Split(strings.TrimRight(resolve.Report, "\n"), "\n"), 20) {
			_, _ = fmt.Fprintln(out, "  "+line)
		}
		if path, err := u.WriteReport(report); err == nil {
			_, _ = fmt.Fprintln(out, tui.DimText.Render("Full output: "+path))
		}
		return fmt.Errorf("composer cannot resolve the upgrade to %s", target.Version)
	}

	_, _ = fmt.Fprintln(out, tui.SuccessLine("Composer can resolve this upgrade."))

	if opts.DryRun {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, tui.SectionHeadingStyle.Render("Planned changes (dry run — nothing modified)"))
		for _, change := range u.PlannedChanges() {
			_, _ = fmt.Fprintln(out, "  • "+change)
		}
		if path, err := u.WriteReport(report); err == nil {
			_, _ = fmt.Fprintln(out, tui.DimText.Render("Report: "+path))
		}
		return nil
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, tui.SectionHeadingStyle.Render("Executing upgrade"))
	runErr := u.consumeRunEvents(out, u.Run(ctx, RunOptions{
		Target:           target.Version.String(),
		ResolvedVersions: resolve.VersionMap(),
		Report:           report,
	}))

	u.trackHeadlessOutcome(catalog.Current.String(), target.Version.String(), runErr)

	if runErr != nil {
		_, _ = fmt.Fprintln(out, tui.FailLine("Upgrade failed and was rolled back."))
		_, _ = fmt.Fprintln(out, tui.DimText.Render("Log: "+u.LogPath()))
		return runErr
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, tui.SuccessLine(fmt.Sprintf("Upgraded to Shopware %s.", target.Version)))
	_, _ = fmt.Fprintln(out, tui.DimText.Render("Report: "+u.ReportPath()))
	_, _ = fmt.Fprintln(out, tui.DimText.Render("Log:    "+u.LogPath()))
	_, _ = fmt.Fprintln(out, tui.DimText.Render("Verify the shop, run your test suite, then commit composer.json and composer.lock."))
	return nil
}

// selectTarget resolves the --target value against the catalog. An empty or
// unknown target fails with the list of supported versions, so unattended
// runs never jump majors by accident.
func selectTarget(catalog *Catalog, target string) (*VersionOption, error) {
	switch target {
	case "":
		return nil, fmt.Errorf("--target is required in non-interactive mode; available versions:\n%s", availableTargets(catalog))
	case TargetRecommended:
		if catalog.Recommended < 0 {
			return nil, fmt.Errorf("no recommended version available")
		}
		return &catalog.Options[catalog.Recommended], nil
	case TargetLatestPatch:
		if catalog.LatestPatch < 0 {
			return nil, fmt.Errorf("no newer patch release of the current minor available")
		}
		return &catalog.Options[catalog.LatestPatch], nil
	}

	for i, opt := range catalog.Options {
		if opt.Version.String() == strings.TrimPrefix(target, "v") {
			return &catalog.Options[i], nil
		}
	}
	return nil, fmt.Errorf("%q is not a supported upgrade target; available versions:\n%s", target, availableTargets(catalog))
}

func availableTargets(catalog *Catalog) string {
	var b strings.Builder
	for _, opt := range catalog.Options {
		_, _ = fmt.Fprintf(&b, "  %s", opt.Version)
		if opt.Tag != "" {
			_, _ = fmt.Fprintf(&b, "  (%s)", opt.Tag)
		}
		b.WriteString("\n")
	}
	b.WriteString("or use --target " + TargetRecommended + " / --target " + TargetLatestPatch)
	return b.String()
}

// resolveHeadless runs the Composer dry-run. When packages are blocked by
// security advisories and the user has not opted out of audit blocking
// (--no-audit disables it before the run), it fails with the same hint
// project create prints.
func (u *ProjectUpgrader) resolveHeadless(ctx context.Context, target string) (ResolveResult, error) {
	resolve, err := u.CheckComposerResolvable(ctx, target)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("composer resolution check: %w", err)
	}

	if resolve.SecurityBlocked() && !u.AuditBlockDisabled() {
		return ResolveResult{}, fmt.Errorf("dependencies of Shopware %s are affected by known security advisories; re-run with --no-audit to proceed. We strongly recommend installing the Shopware Security plugin (https://store.shopware.com/en/swag136939272659f/shopware-6-security-plugin.html) which backports security fixes to older versions", target)
	}
	return resolve, nil
}

func printExtensionResults(out io.Writer, results []ExtensionResult) {
	if len(results) == 0 {
		return
	}

	rows := make([][]string, 0, len(results))
	for _, r := range results {
		available := r.Available
		if available == "" {
			available = "-"
		}
		rows = append(rows, []string{r.Extension.Name, r.Extension.Version, available, r.Status.Label()})
	}
	_, _ = fmt.Fprintln(out, tui.RenderTable([]string{"Extension", "Current", "Target", "Result"}, rows))
}

// headlessReportData assembles the report for a headless run — the same data
// the wizard's review panel gathers.
func (u *ProjectUpgrader) headlessReportData(ctx context.Context, readiness Readiness, catalog *Catalog, target *VersionOption, results []ExtensionResult, resolve ResolveResult) ReportData {
	composerReport := ""
	if !resolve.OK {
		composerReport = resolve.Report
	}

	return ReportData{
		ProjectName:     filepath.Base(u.projectRoot),
		Current:         catalog.Current.String(),
		Target:          target.Version.String(),
		GeneratedAt:     time.Now(),
		Checks:          readiness.Checks,
		Extensions:      results,
		PlannedChanges:  u.PlannedChanges(),
		PHPRequirement:  u.TargetPHPRequirement(ctx, target.Version),
		PHPInstalled:    u.InstalledPHPVersion(ctx),
		ComposerReport:  composerReport,
		ResolvedChanges: resolve.Changes,
	}
}

// consumeRunEvents streams runner progress to out and returns the final error.
func (u *ProjectUpgrader) consumeRunEvents(out io.Writer, events <-chan StepEvent) error {
	var runErr error
	for ev := range events {
		switch {
		case ev.Line != "":
			_, _ = fmt.Fprintln(out, "  "+ev.Line)
		case ev.Step == StepFinished:
			if ev.State == StateFail {
				runErr = ev.Err
			}
		case ev.State == StateRunning:
			_, _ = fmt.Fprintln(out, tui.BoldText.Render("▸ "+ev.Step.Label()))
		case ev.State == StateOK:
			_, _ = fmt.Fprintf(out, "%s %s\n", tui.CheckOK, ev.Step.Label())
		case ev.State == StateWarn:
			_, _ = fmt.Fprintf(out, "%s %s: %v\n", tui.CheckWarn, ev.Step.Label(), ev.Err)
		case ev.State == StateFail:
			_, _ = fmt.Fprintf(out, "%s %s: %v\n", tui.CheckFail, ev.Step.Label(), ev.Err)
		}
	}
	return runErr
}

// PlannedChanges lists what the runner will do — shown on the review panel,
// in dry runs, and in the report.
func (u *ProjectUpgrader) PlannedChanges() []string {
	changes := []string{"update composer.json"}
	if u.AuditBlockDisabled() {
		changes = append(changes, "run Composer with security blocking disabled (COMPOSER_NO_SECURITY_BLOCKING=1)")
	}
	return append(changes,
		"composer update --with-all-dependencies",
		"composer recipes:install --force --reset",
		"update composer.lock",
		"write .shopware-cli/upgrade/report.md",
	)
}

func (u *ProjectUpgrader) trackHeadlessOutcome(current, target string, runErr error) {
	result := tracking.ResultSuccess
	if runErr != nil {
		result = tracking.ResultFailure
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	tracking.Track(ctx, tracking.EventProjectUpgrade, map[string]string{
		tracking.TagFromVersion:   current,
		tracking.TagTargetVersion: target,
		tracking.TagResult:        result,
	})
}

func stateGlyph(s CheckState) string {
	switch s {
	case StateOK:
		return tui.CheckOK
	case StateWarn:
		return tui.CheckWarn
	case StateFail:
		return tui.CheckFail
	case StateRunning, StatePending:
		return tui.StateDot(tui.DotPending)
	}
	return tui.StateDot(tui.DotPending)
}
