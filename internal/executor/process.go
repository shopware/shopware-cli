package executor

import (
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/shopware/shopware-cli/internal/oci"
)

type Process struct {
	Cmd  oci.Cmd
	stop func(ctx context.Context) error
}

func (p *Process) Stop(ctx context.Context) error {
	if p.stop != nil {
		return p.stop(ctx)
	}

	if proc := p.Cmd.Process(); proc != nil {
		return proc.Signal(syscall.SIGINT)
	}

	return nil
}

func (p *Process) Run() error {
	return p.Cmd.Run()
}

// RunWithOutput runs the command and streams its combined stdout/stderr to w.
func (p *Process) RunWithOutput(w io.Writer) error {
	p.Cmd.SetStdout(w)
	p.Cmd.SetStderr(w)
	return p.Cmd.Run()
}

// StartCombined starts the command with its stdout and stderr merged into a
// single reader. Unlike RunWithOutput it does not block, and the returned reader
// reaches EOF once the process (and the read end of the pipe) is done, so a
// reader does not hang when the process is signaled to stop.
func (p *Process) StartCombined() (io.ReadCloser, error) {
	stdout, err := p.Cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p.Cmd.SetStderr(p.Cmd.Stdout())

	if err := p.Cmd.Start(); err != nil {
		return nil, err
	}

	return stdout, nil
}

func (p *Process) Output() ([]byte, error) {
	return p.Cmd.Output()
}

func (p *Process) CombinedOutput() ([]byte, error) {
	return p.Cmd.CombinedOutput()
}

func (p *Process) Start() error {
	return p.Cmd.Start()
}

func (p *Process) Wait() error {
	return p.Cmd.Wait()
}

func (p *Process) StdoutPipe() (io.ReadCloser, error) {
	return p.Cmd.StdoutPipe()
}

func (p *Process) StderrPipe() (io.ReadCloser, error) {
	return p.Cmd.StderrPipe()
}

func newProcess(cmd *exec.Cmd) *Process {
	return &Process{Cmd: oci.WrapCommand(cmd)}
}
