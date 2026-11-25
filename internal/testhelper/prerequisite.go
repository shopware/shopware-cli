package testhelper

import (
	"os"
	"os/exec"
	"testing"
)

// MissingPrerequisitef fails the test in CI and skips it locally.
func MissingPrerequisitef(tb testing.TB, format string, args ...any) {
	tb.Helper()
	if os.Getenv("CI") == "true" {
		tb.Fatalf(format, args...)
	}
	tb.Skipf(format, args...)
}

// RequireTool returns the path to a test dependency.
func RequireTool(tb testing.TB, name string) string {
	tb.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		MissingPrerequisitef(tb, "cannot find %s: %v", name, err)
	}
	return path
}
