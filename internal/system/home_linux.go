package system

import (
	"os"
	"syscall"
)

// EnsureWritableHome points HOME at a writable directory when the current HOME
// is unset or not writable. This happens when shopware-cli runs inside a
// container as a mapped host UID with no passwd entry: HOME resolves to "/",
// where tools like npm (cache ~/.npm) and composer (~/.composer) fail with
// EACCES. On a normal host with a writable HOME it is a no-op. Returns the
// directory HOME was redirected to, or "" when nothing was changed.
//
// Linux only: macOS and Windows do not expose this problem (Docker Desktop
// handles bind-mount ownership) and use different home conventions; see the
// no-op in home_other.go.
func EnsureWritableHome() string {
	if home := os.Getenv("HOME"); home != "" && isWritableDir(home) {
		return ""
	}

	fallback := os.TempDir()
	if !isWritableDir(fallback) {
		// Nothing better to offer; let the original error surface instead of
		// pointing HOME at another unwritable location.
		return ""
	}

	if err := os.Setenv("HOME", fallback); err != nil {
		return ""
	}

	return fallback
}

// isWritableDir reports whether the process can create entries in dir. It uses
// access(2) so it performs no write and leaves no probe file behind (run() calls
// EnsureWritableHome on every invocation, including shell completion).
func isWritableDir(dir string) bool {
	const writeOK = 0x2 // unix W_OK
	return syscall.Access(dir, writeOK) == nil
}
