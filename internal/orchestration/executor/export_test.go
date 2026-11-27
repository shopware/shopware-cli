package executor

import (
	"testing"
	"time"
)

var ErrVersionProbeTimeout = errVersionProbeTimeout

// SetVersionProbeTimeout shortens the version probe timeout for one test.
func SetVersionProbeTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()

	previous := versionProbeTimeout
	t.Cleanup(func() { versionProbeTimeout = previous })
	versionProbeTimeout = timeout
}

// SetCaptureLimit lowers the capture limit for one test.
func SetCaptureLimit(t *testing.T, limit int) {
	t.Helper()

	previous := captureLimit
	t.Cleanup(func() { captureLimit = previous })
	captureLimit = limit
}
