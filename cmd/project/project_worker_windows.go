//go:build windows

package project

import "os"

// gracefulStop kills the process immediately. Windows console child processes cannot receive SIGTERM, so graceful termination is not possible;
func gracefulStop(proc *os.Process, _ uint) error {
	return proc.Kill()
}
