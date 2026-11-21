package system

import (
	"context"
	"os/exec"
	"time"
)

// CommandContext stops the process group on cancellation where supported and
// bounds the wait for descendants that keep output pipes open.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	configureCommandCancellation(cmd)
	return cmd
}
