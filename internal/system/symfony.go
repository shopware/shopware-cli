package system

import "os/exec"

func IsSymfonyCliInstalled() bool {
	_, err := exec.LookPath("symfony")
	return err == nil
}
