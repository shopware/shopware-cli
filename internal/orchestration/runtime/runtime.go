// Package runtime defines command execution contracts for target environments.
package runtime

import (
	"context"
	"io"
	"time"
)

// Command describes a process without implicit shell parsing or path rewriting.
type Command struct {
	// Name identifies the executable in the target environment.
	Name string
	// Args contains the arguments passed to the executable.
	Args []string
	// Dir is the working directory in the target environment.
	Dir string
	// Env overrides environment values in the target environment.
	Env map[string]string
}

// Runtime starts commands in a target environment. Implementations must support
// concurrent starts and must not modify the supplied command or options.
type Runtime interface {
	// Start returns a process once started, or a nil process and an error.
	// Its context controls execution lifetime. Cancellation requests forceful
	// termination. The runtime reaps the process even if Wait is never called.
	// Runtimes forward streams, and the executor implements output capture.
	Start(context.Context, Command, Options) (Process, error)
}

// Options configures process input, output and terminal handling.
type Options struct {
	// Stdin supplies input. Nil means no input. The caller must unblock a reader
	// that remains blocked after the process exits.
	Stdin io.Reader
	// Stdout receives standard output. Nil discards streamed output.
	Stdout io.Writer
	// Stderr receives standard error. Nil discards streamed output.
	Stderr io.Writer
	// Capture selects the output retained by the executor in Result.Output.
	// The executor consumes it before dispatch, so runtimes receive CaptureNone.
	Capture CaptureMode
	// TTY requests terminal allocation. Unsupported settings return ErrUnsupported.
	TTY bool
}

// CaptureMode selects which process output the executor retains.
type CaptureMode uint8

const (
	// CaptureNone disables output retention.
	CaptureNone CaptureMode = iota
	// CaptureSeparate retains standard output and standard error separately.
	CaptureSeparate
	// CaptureCombined merges both streams. The write order is exact when both
	// streams use the same writer or none. Otherwise it is approximate.
	CaptureCombined
)

// SameWriter reports whether stdout and stderr use one writer, so a runtime can
// forward both through one stream and keep their order. Writers that cannot be
// compared count as different, as in os/exec.
func SameWriter(stdout, stderr io.Writer) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()

	return stdout == stderr
}

// Process observes completion and requests termination of a started command.
// Its methods may be called concurrently.
type Process interface {
	// Wait waits for exit, output forwarding and cleanup. Repeated or concurrent
	// calls observe the same outcome. Each result owns its output and usage data.
	// The error reports only the runtime's own failures, such as forwarding,
	// cleanup or cancellation. The workload's exit is reported only through
	// Result.ExitCode, which is -1 when terminated without an exit code or when
	// the status is unknown. The executor turns an unsuccessful exit into an
	// ExitError. On error, the result still contains all available data.
	// Caller-owned streams must remain usable until completion. Blocked output
	// writes can delay completion. A runtime may bound that delay by discarding
	// the remaining output and reporting an error.
	Wait() (Result, error)
	// Stop requests graceful termination without waiting for exit. A canceled
	// context prevents sending the request. There is no automatic escalation.
	// Its context bounds communication with the runtime, not process lifetime.
	// Unsupported termination returns ErrUnsupported. Use Wait to observe exit.
	Stop(context.Context) error
	// Kill requests forceful termination without waiting for exit. Its context
	// bounds communication, not process lifetime. A canceled context prevents
	// sending the request. Unsupported termination returns ErrUnsupported.
	// Use Wait to observe exit and cleanup.
	Kill(context.Context) error
}

// Result contains available workload data, including when execution returns an error.
type Result struct {
	// ExitCode is the workload's exit code, or -1 when unknown. Implementations
	// must set -1 explicitly when status is unavailable, because the zero value means 0.
	ExitCode int
	// Signal is the name of the signal that ended the workload, for example
	// SIGKILL. It is empty when the workload was not ended by a signal or when
	// that is unknown.
	Signal string
	// Output contains the output retained according to Options.Capture.
	Output CapturedOutput
	// Usage describes workload resource use. Nil means unavailable.
	Usage *ResourceUsage
}

// ResourceUsage contains measurements for the executed workload.
// It excludes the Docker or SSH client used to launch or communicate with it.
type ResourceUsage struct {
	// CPUTime is the workload's accumulated CPU time.
	CPUTime time.Duration
}

// CapturedOutput retains process output according to its capture mode.
type CapturedOutput struct {
	// Mode identifies the capture mode used for execution.
	Mode CaptureMode
	// Stdout contains standard output in separate mode.
	Stdout []byte
	// Stderr contains standard error in separate mode.
	Stderr []byte
	// Combined contains both streams in combined mode, see CaptureCombined.
	Combined []byte
}
