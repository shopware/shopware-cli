package executor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// versionProbeTimeout bounds a version probe, because a broken installation can
// hang. It is a variable so tests can shorten it.
var versionProbeTimeout = 10 * time.Second

var errVersionProbeTimeout = errors.New("the version command did not answer in time")

// probeVersion runs a tool's version command and returns the version that the
// first group of pattern captures from its standard output.
func probeVersion(ctx context.Context, cmd *Command, tool string, pattern *regexp.Regexp) (*version.Version, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, versionProbeTimeout, errVersionProbeTimeout)
	defer cancel()

	result, err := cmd.Run(ctx, runtime.Options{Capture: runtime.CaptureSeparate})
	stdout := strings.TrimSpace(string(result.Output.Stdout))

	if err != nil {
		if errors.Is(context.Cause(ctx), errVersionProbeTimeout) {
			err = errVersionProbeTimeout
		}

		if output := cmp.Or(strings.TrimSpace(string(result.Output.Stderr)), stdout); output != "" {
			err = fmt.Errorf("%w, output: %s", err, output)
		}

		return nil, fmt.Errorf("cannot check installed %s version: %w", tool, err)
	}

	matches := pattern.FindStringSubmatch(stdout)
	if matches == nil {
		return nil, fmt.Errorf("cannot parse installed %s version: unexpected output %q", tool, stdout)
	}

	installed, err := version.NewVersion(matches[1])
	if err != nil {
		return nil, fmt.Errorf("cannot parse installed %s version: %w", tool, err)
	}

	return installed, nil
}
