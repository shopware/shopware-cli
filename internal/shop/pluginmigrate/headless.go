package pluginmigrate

import (
	"context"
	"fmt"
	"io"

	"github.com/shopware/shopware-cli/internal/tui"
)

// HeadlessOptions configure a non-interactive migration run.
type HeadlessOptions struct {
	// Token is the Shopware Packagist token (SHOPWARE_PACKAGIST_TOKEN).
	// Empty is allowed: every extension is then managed via a path
	// repository instead of being required from the Store.
	Token string
	// DryRun stops after printing the plan without modifying any file.
	DryRun bool
	// Out receives the progress output.
	Out io.Writer
}

// RunHeadless migrates the project's custom/ extensions without the TUI —
// for --no-interaction runs and environments without a terminal.
func (m *PluginMigrator) RunHeadless(ctx context.Context, opts HeadlessOptions) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	extensions := m.Scan(ctx)
	if len(extensions) == 0 {
		_, _ = fmt.Fprintln(out, tui.SuccessLine("All extensions are already managed through Composer — nothing to do."))
		return nil
	}

	if opts.Token == "" {
		_, _ = fmt.Fprintln(out, tui.DimText.Render("No SHOPWARE_PACKAGIST_TOKEN set — skipping the Shopware Store; Packagist and configured repositories are still checked."))
	}
	avail, err := m.FetchAvailability(ctx, opts.Token, extensions)
	if err != nil {
		return fmt.Errorf("fetch Shopware Packagist packages (is SHOPWARE_PACKAGIST_TOKEN valid?): %w", err)
	}

	plan := BuildPlan(extensions, avail)
	printPlan(out, plan)

	if !plan.Actionable() {
		return fmt.Errorf("none of the extensions can be migrated automatically; add a composer.json with a package name to each")
	}

	if opts.DryRun {
		_, _ = fmt.Fprintln(out, tui.DimText.Render("Dry run — nothing was modified."))
		return nil
	}

	_, _ = fmt.Fprintln(out)
	runErr := consumeRunEvents(out, m.Run(ctx, plan, opts.Token))
	if runErr != nil {
		_, _ = fmt.Fprintln(out, tui.FailLine("Migration failed; composer.json, composer.lock, and auth.json were restored."))
		return runErr
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, tui.SuccessLine("All extensions are now managed through Composer."))
	_, _ = fmt.Fprintln(out, tui.DimText.Render("Verify the shop, then commit composer.json, composer.lock, and auth.json."))
	return nil
}

func printPlan(out io.Writer, plan Plan) {
	rows := make([][]string, 0, len(plan.Extensions))
	for _, ext := range plan.Extensions {
		version := ext.Version
		if version == "" {
			version = "-"
		}
		rows = append(rows, []string{ext.Name, version, ext.Kind.Label()})
	}
	_, _ = fmt.Fprintln(out, tui.RenderTable([]string{"Extension", "Version", "Action"}, rows))
}

func consumeRunEvents(out io.Writer, events <-chan StepEvent) error {
	var runErr error
	for ev := range events {
		if ev.Step == StepFinished && ev.State == StateFail {
			runErr = ev.Err
		}
		switch {
		case ev.Line != "":
			_, _ = fmt.Fprintln(out, "  "+ev.Line)
		case ev.Step == StepFinished:
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
