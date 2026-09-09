package cmd

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
)

func trackCommandExecution(ctx context.Context, args []string, start time.Time, runErr error) {
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == rootCmd || cmd.RunE == nil {
		return
	}

	result := tracking.ResultSuccess
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			result = tracking.ResultCancelled
		} else {
			result = tracking.ResultFailure
		}
	}
	name := strings.TrimPrefix(cmd.CommandPath(), "shopware-cli ")
	name = strings.ReplaceAll(name, " ", ".")
	name = strings.ReplaceAll(name, "-", "_")
	trackCtx, trackCancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
	defer trackCancel()
	tracking.Track(trackCtx, tracking.EventCommand, map[string]string{
		tracking.TagCommandName: name,
		tracking.TagResult:      result,
		tracking.TagDurationMS:  strconv.FormatInt(time.Since(start).Milliseconds(), 10),
		tracking.TagCLIVersion:  version,
		tracking.TagOS:          runtime.GOOS,
		tracking.TagIsTUI:       strconv.FormatBool(system.IsInteractionEnabled(ctx)),
	})
}
