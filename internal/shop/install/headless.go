package install

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
)

// HeadlessOptions configure a non-interactive installation run.
type HeadlessOptions struct {
	// Install parameterizes the installation; empty fields get the wizard
	// defaults.
	Install Options
	// Out receives the progress output.
	Out io.Writer
}

// RunHeadless installs Shopware without the TUI — for the
// `project dev install` command and environments without a terminal. An
// already-installed shop is reported and skipped, so the command is
// idempotent for scripts and CI.
func RunHeadless(ctx context.Context, exec executor.Executor, cfg *shop.Config, envCfg *shop.EnvironmentConfig, projectRoot string, opts HeadlessOptions) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	install := opts.Install
	install.ApplyDefaults()
	if err := install.Validate(); err != nil {
		return err
	}

	if IsInstalled(ctx, exec) {
		_, _ = fmt.Fprintln(out, tui.SuccessLine("Shopware is already installed — nothing to do."))
		_, _ = fmt.Fprintln(out, tui.DimText.Render("Run ")+tui.BoldText.Render("shopware-cli project dev stop --remove-data")+tui.DimText.Render(" first for a clean reinstall."))
		trackOutcome(tracking.ResultSkipped, install, 0, "")
		return nil
	}

	_, _ = fmt.Fprintln(out, tui.SectionHeadingStyle.Render("Installing Shopware"))

	start := time.Now()
	currentStep := -1
	runErr := Run(ctx, exec, install, func(line string) {
		if idx, ok := MatchStep(line, currentStep+1); ok {
			currentStep = idx
			_, _ = fmt.Fprintln(out, tui.BoldText.Render("▸ "+Steps[idx].Label))
		}
		_, _ = fmt.Fprintln(out, "  "+tui.DimText.Render(line))
	})
	elapsed := time.Since(start)

	if runErr != nil {
		failedStep := FailedStep(max(currentStep, 0))
		trackOutcome(tracking.ResultFailure, install, elapsed, failedStep)
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, tui.FailLine("Installation failed during "+failedStep+"."))
		return fmt.Errorf("installing Shopware: %w", runErr)
	}

	// Persist before recording the outcome, so a failed config write is not
	// reported as a successful run.
	if err := PersistCredentials(cfg, envCfg, projectRoot, install); err != nil {
		trackOutcome(tracking.ResultFailure, install, elapsed, FailedStepSaveCredentials)
		return fmt.Errorf("installation succeeded, but failed to save admin credentials to the project config: %w", err)
	}

	trackOutcome(tracking.ResultSuccess, install, elapsed, "")

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, tui.SuccessLine(fmt.Sprintf("Shopware installed in %s", elapsed.Round(time.Second))))

	shopURL := envCfg.URL
	if shopURL == "" {
		shopURL = cfg.URL
	}
	if shopURL != "" {
		_, _ = fmt.Fprintln(out, tui.DimText.Render("Admin URL: ")+tui.BoldText.Render(strings.TrimSuffix(shopURL, "/")+"/admin"))
	}
	_, _ = fmt.Fprintln(out, tui.DimText.Render("Admin user: ")+tui.BoldText.Render(install.AdminUsername)+tui.DimText.Render(" (credentials saved to "+shop.DefaultConfigFileName()+")"))

	return nil
}

// trackOutcome sends the project.dev.install event for a headless run. Unlike
// the TUI it sends synchronously (with a short timeout), because the process
// exits right after.
func trackOutcome(result string, opts Options, duration time.Duration, failedStep string) {
	tags := map[string]string{
		tracking.TagResult:            result,
		tracking.TagLanguage:          opts.Locale,
		tracking.TagCurrency:          opts.Currency,
		tracking.TagCustomCredentials: strconv.FormatBool(opts.CustomCredentials()),
		tracking.TagInteractive:       "false",
	}
	if duration > 0 {
		tags[tracking.TagDurationMS] = strconv.FormatInt(duration.Milliseconds(), 10)
	}
	if failedStep != "" {
		tags[tracking.TagFailedStep] = failedStep
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	tracking.Track(ctx, tracking.EventDevInstall, tags)
}
