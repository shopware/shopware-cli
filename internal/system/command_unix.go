//go:build unix

package system

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureCommandCancellation gives the command its own process group so context
// cancellation kills the command and workers in that group together.
func configureCommandCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
