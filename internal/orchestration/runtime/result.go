package runtime

import "time"

// Result contains the workload exit code and any captured output.
type Result struct {
	// ExitCode is the process exit code, or -1 when it is unknown.
	ExitCode int

	// Output contains captured output when capture was requested.
	Output CapturedOutput

	// Usage contains workload resource usage, or nil when it is unavailable.
	Usage *ResourceUsage
}

// ResourceUsage reports resource consumption measured by a backend.
type ResourceUsage struct {
	// CPUTime is total user and system CPU time. Child process coverage depends
	// on the backend and operating system.
	CPUTime time.Duration
}

// CaptureMode selects which process output is retained.
type CaptureMode uint8

const (
	// CaptureNone leaves output uncaptured.
	CaptureNone CaptureMode = iota

	// CaptureSeparate retains stdout and stderr separately.
	CaptureSeparate

	// CaptureCombined retains both streams in observed write order.
	CaptureCombined
)

// CapturedOutput contains the output retained in the requested mode.
type CapturedOutput struct {
	// Mode identifies the captured representation.
	Mode CaptureMode

	// Stdout contains standard output in separate mode.
	Stdout []byte

	// Stderr contains standard error in separate mode.
	Stderr []byte

	// Combined contains both streams in combined mode.
	Combined []byte
}
