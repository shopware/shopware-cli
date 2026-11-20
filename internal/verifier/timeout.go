package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
)

// DefaultQATimeout disables the QA deadline unless a budget is configured.
const DefaultQATimeout time.Duration = 0

// QATimeoutEnv overrides the default budget when no flag is supplied.
const QATimeoutEnv = "SHOPWARE_CLI_QA_TIMEOUT"

// Known QA phase identifiers used in telemetry.
const (
	QAPhaseExtensionDiscovery     = "extension_discovery"
	QAPhaseProjectDiscovery       = "project_discovery"
	QAPhaseConfigurationDiscovery = "configuration_discovery"
	QAPhaseArchiveExtraction      = "archive_extraction"
	QAPhaseToolSetup              = "tool_setup"
	QAPhaseSetupAndDiscovery      = "tool_setup_and_configuration_discovery"
	QAPhaseExecution              = "execution"
)

// ErrQATimeout identifies expiry of the QA budget.
var ErrQATimeout = fmt.Errorf("QA operation timed out: %w", context.DeadlineExceeded)

// resolveQATimeout selects the explicit flag, environment value, or default in that order.
func resolveQATimeout(flagValue string, flagSet bool, envValue string) (time.Duration, error) {
	value := flagValue
	if !flagSet {
		if envValue == "" {
			return DefaultQATimeout, nil
		}
		value = envValue
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("cannot parse QA timeout %q: %w", value, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("cannot use negative QA timeout %q", value)
	}

	return d, nil
}

// WithQATimeout starts the configured budget, or leaves the parent unchanged when disabled.
func WithQATimeout(parent context.Context, value string, explicit bool) (context.Context, context.CancelFunc, error) {
	budget, err := resolveQATimeout(value, explicit, os.Getenv(QATimeoutEnv))
	if err != nil {
		return nil, nil, err
	}

	tracking.SetCommandTag(tracking.TagQATimeoutMS, strconv.FormatInt(budget.Milliseconds(), 10))
	if budget == 0 {
		return parent, func() {}, nil
	}

	ctx, cancel := context.WithTimeoutCause(parent, budget, fmt.Errorf("%w after %s", ErrQATimeout, budget))
	return ctx, cancel, nil
}

// QAPhaseError records a QA timeout phase and preserves the original error.
// Other cancellation causes leave the error unchanged.
func QAPhaseError(ctx context.Context, phase string, err error) error {
	cause := context.Cause(ctx)
	if !errors.Is(cause, ErrQATimeout) {
		return err
	}

	tracking.SetCommandTag(tracking.TagQATimeoutPhase, phase)
	label := strings.ReplaceAll(phase, "_", " ")
	return fmt.Errorf("cannot complete %s: %w", label, errors.Join(cause, err))
}

// EnsureQATimeoutResult records an unreported QA deadline while preserving tool reports.
// The caller must cancel the QA context before calling so its cause is fixed.
func EnsureQATimeoutResult(ctx context.Context, reports []validation.ToolReport) []validation.ToolReport {
	cause := context.Cause(ctx)
	if !errors.Is(cause, ErrQATimeout) {
		return reports
	}

	for _, report := range reports {
		for _, err := range report.Errors {
			if errors.Is(err, ErrQATimeout) {
				return reports
			}
		}
	}

	tracking.SetCommandTag(tracking.TagQATimeoutPhase, QAPhaseExecution)
	return append(reports, validation.ToolReport{
		Name:   "QA",
		Errors: []validation.ToolError{{Err: cause}},
	})
}
