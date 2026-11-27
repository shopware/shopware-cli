package executor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// captureLimit bounds the bytes retained across both streams of one command.
// It is a variable so tests can lower it.
var captureLimit = 128 << 20

// ErrCaptureLimit reports that a command wrote more output than is retained.
// The captured output is cut off at the limit. Forwarding is not affected.
var ErrCaptureLimit = errors.New("captured output exceeds the limit")

// capture retains output from both streams for one command invocation.
type capture struct {
	// One lock preserves observed write order and protects shared stream writers.
	mu       sync.Mutex
	mode     runtime.CaptureMode
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	combined bytes.Buffer
	retained int
	exceeded bool
}

// captureOptions routes both streams through capture writers, so the runtime
// only forwards output. It returns a nil capture when capture is disabled.
func captureOptions(options runtime.Options) (runtime.Options, *capture, error) {
	switch options.Capture {
	case runtime.CaptureNone:
		return options, nil, nil
	case runtime.CaptureSeparate, runtime.CaptureCombined:
	default:
		return options, nil, fmt.Errorf("cannot capture process output: invalid capture mode %d", options.Capture)
	}

	output := &capture{mode: options.Capture}
	stdout := &captureWriter{capture: output, target: options.Stdout}
	stderr := &captureWriter{capture: output, target: options.Stderr, stderr: true}
	if options.Capture == runtime.CaptureCombined && runtime.SameWriter(options.Stdout, options.Stderr) {
		// One writer for both streams lets the runtime keep their order.
		stderr = stdout
	}

	// The writers capture the output, so the runtime only forwards the streams.
	options.Capture = runtime.CaptureNone
	options.Stdout, options.Stderr = stdout, stderr

	return options, output, nil
}

// captureWriter retains one stream and optionally forwards it to a caller's writer.
// Writers for the same invocation share capture state and synchronization.
type captureWriter struct {
	capture *capture
	target  io.Writer
	stderr  bool
}

// Write captures data before forwarding it and returns the forwarding result.
// Observed bytes are retained up to the capture limit, even if forwarding fails
// partway through.
// A nil target disables forwarding. Blocked forwarding holds the shared lock,
// delaying writes to either stream and completion of the command.
func (w *captureWriter) Write(data []byte) (int, error) {
	w.capture.mu.Lock()
	defer w.capture.mu.Unlock()

	kept := data[:min(len(data), captureLimit-w.capture.retained)]
	w.capture.retained += len(kept)
	w.capture.exceeded = w.capture.exceeded || len(kept) < len(data)

	switch {
	case w.capture.mode == runtime.CaptureCombined:
		_, _ = w.capture.combined.Write(kept)
	case w.stderr:
		_, _ = w.capture.stderr.Write(kept)
	default:
		_, _ = w.capture.stdout.Write(kept)
	}

	if w.target == nil {
		return len(data), nil
	}

	return w.target.Write(data)
}

// snapshot returns an independent copy of the captured output, and
// ErrCaptureLimit when output was cut off.
func (c *capture) snapshot() (runtime.CapturedOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	output := runtime.CapturedOutput{
		Mode:     c.mode,
		Stdout:   bytes.Clone(c.stdout.Bytes()),
		Stderr:   bytes.Clone(c.stderr.Bytes()),
		Combined: bytes.Clone(c.combined.Bytes()),
	}
	if c.exceeded {
		return output, ErrCaptureLimit
	}

	return output, nil
}
