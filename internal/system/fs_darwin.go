//go:build darwin

package system

import "golang.org/x/sys/unix"

// cloneFile creates a copy-on-write clone of src at dst using clonefile(2).
// It returns nil on success so the caller can skip the regular copy.
// Any error signals the caller to fall back to a regular byte copy
// (e.g. cross-device, non-APFS filesystem, dst on unsupported volume).
func cloneFile(src, dst string) error {
	return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
}
