package system

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL opens the given URL in the user's default browser.
func OpenURL(ctx context.Context, url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	return nil
}
