package cmd

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
)

var trackEvent = tracking.Track

func trackCommandExecution(ctx context.Context, args []string, start time.Time, runErr error) {
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == rootCmd || cmd.RunE == nil {
		return
	}

	// An interrupted run is cancelled even when a child process reported the error.
	result := tracking.ResultSuccess
	switch {
	case runErr == nil:
	case ctx.Err() != nil || errors.Is(runErr, context.Canceled):
		result = tracking.ResultCancelled
	default:
		result = tracking.ResultFailure
	}
	name := strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name()+" ")
	name = strings.ReplaceAll(name, " ", ".")
	name = strings.ReplaceAll(name, "-", "_")
	trackCtx, trackCancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
	defer trackCancel()
	tags := map[string]string{
		tracking.TagCommandName: name,
		tracking.TagResult:      result,
		tracking.TagDurationMS:  strconv.FormatInt(time.Since(start).Milliseconds(), 10),
		tracking.TagOS:          runtime.GOOS,
		tracking.TagIsTUI:       strconv.FormatBool(system.IsInteractionEnabled(ctx)),
		tracking.TagIsWSL:       strconv.FormatBool(system.IsWSL()),
	}
	if provider := system.CIProvider(os.Getenv); provider != "" {
		tags[tracking.TagCIProvider] = provider
	}
	trackEvent(trackCtx, tracking.EventCommand, tags)
}
