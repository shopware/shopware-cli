//go:build !linux

package system

// EnsureWritableHome is a no-op outside Linux. macOS and Windows do not expose
// the unwritable-HOME problem (Docker Desktop handles bind-mount ownership) and
// use different home conventions. See home_linux.go for the Linux behavior.
func EnsureWritableHome() string {
	return ""
}
