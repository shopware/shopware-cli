package system

import "sync/atomic"

var tuiActive atomic.Bool

// SetTUIActive records whether a shopware-cli TUI currently owns the
// terminal. Docker compose exec must keep -T while a TUI is active so it
// does not steal the TTY from the TUI (e.g. project dev).
func SetTUIActive(active bool) {
	tuiActive.Store(active)
}

// IsTUIActive reports whether a TUI currently owns the terminal.
func IsTUIActive() bool {
	return tuiActive.Load()
}
