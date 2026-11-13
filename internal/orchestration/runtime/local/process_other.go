//go:build !darwin && !linux

package local

import (
	"os/exec"
)

func configureProcess(*exec.Cmd) {}

func killProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
