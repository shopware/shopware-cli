package executor

import (
	"context"
	"io"
	"os/exec"
	"syscall"
)

// Process wraps an exec.Cmd and adds a Stop method that knows how to
// terminate the underlying process in an environment-aware way.
// For Docker executors, Stop runs pkill inside the container.
// For local executors, Stop sends SIGINT to the OS process.
type Process struct {
	Cmd  *exec.Cmd
	stop func(ctx context.Context) error
}

// Stop gracefully terminates the process.
func (p *Process) Stop(ctx context.Context) error {
	if p.stop != nil {
		return p.stop(ctx)
	}

	if p.Cmd.Process != nil {
		return p.Cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

// Run starts the command and waits for it to complete.
func (p *Process) Run() error {
	return p.Cmd.Run()
}

// Output runs the command and returns its standard output.
func (p *Process) Output() ([]byte, error) {
	return p.Cmd.Output()
}

// CombinedOutput runs the command and returns its combined standard output and standard error.
func (p *Process) CombinedOutput() ([]byte, error) {
	return p.Cmd.CombinedOutput()
}

// Start starts the specified command but does not wait for it to complete.
func (p *Process) Start() error {
	return p.Cmd.Start()
}

// Wait waits for the command to exit and waits for any copying to stdin or
// copying from stdout or stderr to complete.
func (p *Process) Wait() error {
	return p.Cmd.Wait()
}

// StdoutPipe returns a pipe that will be connected to the command's standard output.
func (p *Process) StdoutPipe() (io.ReadCloser, error) {
	return p.Cmd.StdoutPipe()
}

// StderrPipe returns a pipe that will be connected to the command's standard error.
func (p *Process) StderrPipe() (io.ReadCloser, error) {
	return p.Cmd.StderrPipe()
}

// newProcess creates a Process with the default local stop behavior (SIGINT).
func newProcess(cmd *exec.Cmd) *Process {
	return &Process{Cmd: cmd}
}
