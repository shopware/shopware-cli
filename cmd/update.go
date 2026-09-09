package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/update"
	"github.com/shopware/shopware-cli/logging"
)

// startUpdateCheck shares the background update check with interactive TUIs.
// The caller must cancel the check when command execution finishes.
func startUpdateCheck(ctx context.Context, args []string) (*update.CheckHandle, context.CancelFunc) {
	updateCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
	handle := update.NewCheckHandle()
	tui.UpdateAvailable = func(waitCtx context.Context) bool {
		result := handle.Wait(waitCtx)
		if result.Release == nil {
			return false
		}
		binaryPath, _ := os.Executable()
		return update.ShouldNotify(result.Release, binaryPath)
	}

	go func() {
		var releaseInfo *update.ReleaseInfo
		var err error
		if !update.ShouldCheckForUpdate(version, args) {
			err = update.ErrNoUpdateAvailable
		} else {
			releaseInfo, err = update.CheckForUpdate(updateCtx, version, &http.Client{Timeout: 5 * time.Second})
		}
		if err != nil && !errors.Is(err, update.ErrNoUpdateAvailable) {
			logging.FromContext(ctx).Debugf("checking for shopware cli update failed: %v", err)
		}
		handle.Complete(update.CheckResult{Release: releaseInfo, Err: err})
	}()

	return handle, cancel
}

// printUpdateHint writes the update notification to w when a newer release is
// available and the notification was not already printed recently.
func printUpdateHint(ctx context.Context, w io.Writer, release *update.ReleaseInfo) {
	if release == nil {
		return
	}

	binaryPath, err := os.Executable()
	if err != nil {
		logging.FromContext(ctx).Debugf("could not determine binary path: %v", err)
	}
	if !update.ShouldNotify(release, binaryPath) || !update.ShouldPrintUpdateHint() {
		return
	}

	_, _ = fmt.Fprintln(w, update.RenderUpdateNotification(release.Version, version))
	if err := update.MarkUpdateNotificationPrinted(); err != nil {
		logging.FromContext(ctx).Debugf("could not save update notification timestamp: %v", err)
	}
}
