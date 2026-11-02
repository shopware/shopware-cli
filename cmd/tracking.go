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

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
)

var trackEvent = tracking.Track

func trackCommandExecution(ctx context.Context, args []string, duration time.Duration, runErr error, shopwareVersion string) {
	name, ok := commandEventName(args)
	if !ok {
		return
	}

	// Ctrl+C counts as cancelled, also when a child process or a prompt reported it.
	result := tracking.ResultSuccess
	switch {
	case runErr == nil:
	case ctx.Err() != nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, huh.ErrUserAborted) || errors.Is(runErr, tui.ErrFilterSelectCancelled):
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
	if shopwareVersion != "" {
		tags[tracking.TagShopwareVersion] = shopwareVersion
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
