// Package runtime defines the process execution boundary used by executors.
package runtime

import (
	"context"
	"errors"
	"io"
)

// ErrUnsupported reports a requested runtime feature that a backend cannot provide.
var ErrUnsupported = errors.New("unsupported runtime feature")

// Command describes a program invocation without invoking a shell.
type Command struct {
	// Name is the executable name or path in the target environment.
	Name string

	// Args are passed to the executable without shell parsing.
	Args []string

	// Dir is the working directory in the target environment.
	// An empty value uses the backend's default directory.
	Dir string

	// Env overlays values on the target environment's inherited values.
	Env map[string]string
}

// Options controls the process streams and terminal behavior.
// The caller owns the streams. A blocked output write can delay completion.
// Stdout and Stderr writes are serialized per execution, so they may share a writer.
type Options struct {
	// Stdin supplies process input. A nil reader means no input. The caller owns
	// the reader and must unblock any pending read when it is no longer needed;
	// a blocked read can outlive the process and Wait.
	Stdin io.Reader

	// Stdout receives process standard output. A nil writer discards it.
	Stdout io.Writer

	// Stderr receives process standard error. A nil writer discards it.
	Stderr io.Writer

	// TTY requests terminal allocation.
	TTY bool
}

// Backend starts processes using a specific execution environment.
// The context governs the execution lifetime, including after Start returns.
// Cancellation requests termination; blocked caller-owned I/O can delay completion.
// The backend reaps the process and cleans up execution-owned resources without Wait.
type Backend interface {
	// Start begins a command and returns a handle for waiting or stopping it.
	Start(context.Context, Command, Options) (Process, error)
}

// Process represents a started workload.
type Process interface {
	// Wait blocks until finalization and returns the recorded outcome. It is repeatable.
	// Concurrent callers observe the same completion outcome.
	// The result retains available exit and output data even when an error is returned.
	// A workload failure can be reported as an ExitError.
	Wait() (Result, error)

	// Stop requests forceful termination, even if ctx is already canceled.
	// The context bounds waiting; required cleanup continues independently.
	// A later Stop or Wait can observe the eventual outcome.
	Stop(context.Context) error
}
