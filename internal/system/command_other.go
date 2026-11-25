//go:build !unix

package system

import "os/exec"

// configureCommandCancellation keeps exec.CommandContext's default cancellation
// behavior, which kills only the command process.
func configureCommandCancellation(_ *exec.Cmd) {}
