package executor

import (
	"context"
	"io"
	"os/exec"
	"syscall"
)

type Process struct {
	Cmd  *exec.Cmd
	stop func(ctx context.Context) error
}

func (p *Process) Stop(ctx context.Context) error {
	if p.stop != nil {
		return p.stop(ctx)
	}

	if p.Cmd.Process != nil {
		return p.Cmd.Process.Signal(syscall.SIGINT)
	}

	return nil
}

func (p *Process) Run() error {
	return p.Cmd.Run()
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
	return &Process{Cmd: cmd}
}
