//go:build !windows

package project

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// gracefulStop sends SIGTERM and waits up to gracefulStopLimit seconds for the process to exit before killing it. A limit of 0 kills immediately.
func gracefulStop(proc *os.Process, gracefulStopLimit uint) error {
	if gracefulStopLimit > 0 {
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return err
		}

		deadline := time.Now().Add(time.Second * time.Duration(gracefulStopLimit))
		for time.Now().Before(deadline) {
			if isProcessStopped(proc) {
				return os.ErrProcessDone
			}
			time.Sleep(time.Millisecond * 250)
		}
	}

	return proc.Kill()
}

func isProcessStopped(p *os.Process) bool {
	return errors.Is(p.Signal(syscall.Signal(0)), os.ErrProcessDone)
}
