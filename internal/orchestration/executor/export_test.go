package executor

import "testing"

// SetCaptureLimit lowers the capture limit for one test.
func SetCaptureLimit(t *testing.T, limit int) {
	t.Helper()

	previous := captureLimit
	t.Cleanup(func() { captureLimit = previous })
	captureLimit = limit
}
