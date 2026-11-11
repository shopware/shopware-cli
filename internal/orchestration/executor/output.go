package executor

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

type outputCapture struct {
	// mu serializes capture and forwarding, including a shared output writer.
	mu       sync.Mutex
	mode     runtime.CaptureMode
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	combined bytes.Buffer
}

type captureWriter struct {
	capture *outputCapture
	target  io.Writer
	stderr  bool
}

// Write serializes capture and forwarding, and returns any forwarding error.
func (w captureWriter) Write(p []byte) (int, error) {
	w.capture.mu.Lock()
	defer w.capture.mu.Unlock()

	switch {
	case w.capture.mode == runtime.CaptureCombined:
		_, _ = w.capture.combined.Write(p)
	case w.stderr:
		_, _ = w.capture.stderr.Write(p)
	default:
		_, _ = w.capture.stdout.Write(p)
	}

	if w.target != nil {
		return w.target.Write(p)
	}

	return len(p), nil
}

// snapshot gives each Wait caller its own copy of the captured bytes.
func (c *outputCapture) snapshot() runtime.CapturedOutput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return runtime.CapturedOutput{
		Mode:     c.mode,
		Stdout:   bytes.Clone(c.stdout.Bytes()),
		Stderr:   bytes.Clone(c.stderr.Bytes()),
		Combined: bytes.Clone(c.combined.Bytes()),
	}
}

type capturedProcess struct {
	process runtime.Process
	capture *outputCapture
}

// Wait returns an independent output snapshot even when the process fails.
func (p *capturedProcess) Wait() (runtime.Result, error) {
	result, err := p.process.Wait()
	result.Output = p.capture.snapshot()
	return result, err
}

func (p *capturedProcess) Stop(ctx context.Context) error {
	return p.process.Stop(ctx)
}
