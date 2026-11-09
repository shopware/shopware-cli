package cmd

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/deprecation"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/validation"
)

var trackEvent = tracking.Track

var commandTags = tracking.CommandTags

func trackCommandExecution(ctx context.Context, args []string, duration time.Duration, runErr error, shopwareVersion string) {
	name, ok := commandEventName(args)
	if !ok {
		return
	}

	// Ctrl+C counts as cancelled, also when a child process or a prompt reported it, and so does a declined prompt.
	var declined tracking.Cancelled
	result := tracking.ResultSuccess
	switch {
	case runErr == nil:
	case ctx.Err() != nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, huh.ErrUserAborted) || errors.Is(runErr, tui.ErrFilterSelectCancelled) ||
		errors.As(runErr, &declined):
		result = tracking.ResultCancelled
	default:
		result = tracking.ResultFailure
	}
	trackCtx, trackCancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
	defer trackCancel()
	tags := map[string]string{
		tracking.TagCommandName: name,
		tracking.TagResult:      result,
		tracking.TagDurationMS:  strconv.FormatInt(duration.Milliseconds(), 10),
		tracking.TagOS:          runtime.GOOS,
		tracking.TagIsTUI:       strconv.FormatBool(system.IsInteractionEnabled(ctx)),
		tracking.TagIsWSL:       strconv.FormatBool(system.IsWSL()),
	}
	if provider := system.CIProvider(os.Getenv); provider != "" {
		tags[tracking.TagCIProvider] = provider
	}
	var missing *system.MissingDependenciesError
	var pinnedPHP *system.PHPVersionNotFoundError
	var toolReport *validation.ToolReportError
	switch {
	case result != tracking.ResultFailure:
	case errors.As(runErr, &missing):
		tags[tracking.TagMissing] = strings.Join(missing.IDs(), ",")
	case errors.As(runErr, &pinnedPHP):
		// A pinned PHP that is not installed stops the command before the dependency check.
		tags[tracking.TagMissing] = "php_version"
	case errors.As(runErr, &toolReport):
		if len(toolReport.FailedTools) > 0 {
			tags[tracking.TagFailedTools] = strings.Join(toolReport.FailedTools, ",")
		}
		if len(toolReport.ToolsWithErrorFindings) > 0 {
			tags[tracking.TagToolsWithErrorFindings] = strings.Join(toolReport.ToolsWithErrorFindings, ",")
		}
	}
	var failedStep *tracking.FailedStepError
	if result != tracking.ResultSuccess && errors.As(runErr, &failedStep) {
		tags[tracking.TagFailedStep] = failedStep.Step
	}
	if used := deprecation.Used(); len(used) > 0 {
		tags[tracking.TagDeprecated] = strings.Join(used, ",")
	}
	if shopwareVersion != "" {
		tags[tracking.TagShopwareVersion] = shopwareVersion
	}
	// Tags the command set itself never replace the ones above.
	for key, value := range commandTags() {
		if _, set := tags[key]; !set {
			tags[key] = value
		}
	}
	trackEvent(trackCtx, tracking.EventCommand, tags)
}

// commandEventName returns the dotted name of a tracked invocation.
func commandEventName(args []string) (string, bool) {
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == rootCmd || cmd.RunE == nil {
		return "", false
	}
	name := strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name()+" ")
	name = strings.ReplaceAll(name, " ", ".")
	name = strings.ReplaceAll(name, "-", "_")
	return name, true
}
