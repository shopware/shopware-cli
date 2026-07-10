//go:build windows

package project

import "os/exec"

// gracefulStop kills the process immediately. Windows console child processes cannot receive SIGTERM, so graceful termination is not possible;
func gracefulStop(cmd *exec.Cmd, _ uint) error {
	return cmd.Process.Kill()
}
