//go:build !unix

package verifier

import "os/exec"

func configureToolCancellation(_ *exec.Cmd) {}
