//go:build unix

package verifier

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Tools may launch workers that inherit their output pipes. Kill the entire
// process group on cancellation so those workers cannot outlive the command.
func configureToolCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
