//go:build !darwin && !linux

package local

import (
	"os/exec"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func configureProcess(*exec.Cmd) {}

func stopProcess(*exec.Cmd) error {
	return runtime.ErrUnsupported
}

func killProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
