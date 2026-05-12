package system

import "os/exec"

func IsGitInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}
