//go:build !darwin

package system

import "errors"

var errCloneNotSupported = errors.New("clonefile not supported on this platform")

// cloneFile always fails on non-darwin platforms so the caller
// falls back to a regular byte copy.
func cloneFile(_, _ string) error {
	return errCloneNotSupported
}
