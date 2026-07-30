// Package shell provides helpers for building POSIX shell commands.
package shell

import "strings"

// Quote quotes a string for safe use in a POSIX shell command.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
