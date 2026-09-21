// Package ocitest provides in-memory fakes of the oci.Runtime and oci.Cmd
// interfaces, so tests can drive OCI-runtime code paths without a container
// runtime binary or stub scripts on PATH.
package ocitest

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/shopware/shopware-cli/internal/oci"
)

// Runtime is an in-memory oci.Runtime fake. Every created command is
// appended to Cmds; NewCmd optionally customizes each one (e.g. to dispatch
// canned output based on the arguments).
type Runtime struct {
	BinaryName string
	AvailableV bool

	// NewCmd optionally customizes each created command.
	NewCmd func(c *Cmd)

	Cmds []*Cmd
}

var _ oci.Runtime = (*Runtime)(nil)

func (r *Runtime) Binary() string {
	if r.BinaryName == "" {
		return "docker"
	}
	return r.BinaryName
}

func (r *Runtime) Available() bool { return r.AvailableV }

func (r *Runtime) Command(_ context.Context, args ...string) oci.Cmd {
	c := &Cmd{ArgsV: append([]string{r.Binary()}, args...)}
	if r.NewCmd != nil {
		r.NewCmd(c)
	}
	r.Cmds = append(r.Cmds, c)
	return c
}

func (r *Runtime) ComposeCommand(ctx context.Context, args ...string) oci.Cmd {
	return r.Command(ctx, append([]string{"compose"}, args...)...)
}

// Last returns the most recently created command, or nil when none was.
func (r *Runtime) Last() *Cmd {
	if len(r.Cmds) == 0 {
		return nil
	}
	return r.Cmds[len(r.Cmds)-1]
}

// Cmd is an in-memory oci.Cmd fake. Configuration is recorded in the
// exported fields; the Func hooks customize execution results.
type Cmd struct {
	ArgsV   []string
	DirV    string
	EnvV    []string
	StdinV  io.Reader
	StdoutV io.Writer
	StderrV io.Writer
	ErrV    error

	WaitDelayV time.Duration
	CancelFunc func() error

	// RunFunc, OutputFunc, and CombinedOutputFunc customize the matching
	// execution method; nil means a successful no-op (empty output).
	RunFunc            func(c *Cmd) error
	OutputFunc         func(c *Cmd) ([]byte, error)
	CombinedOutputFunc func(c *Cmd) ([]byte, error)
	StartFunc          func(c *Cmd) error
	WaitFunc           func(c *Cmd) error

	Ran     bool
	Started bool
	Waited  bool
}

var _ oci.Cmd = (*Cmd)(nil)

func (c *Cmd) Args() []string { return c.ArgsV }

func (c *Cmd) Dir() string { return c.DirV }

func (c *Cmd) SetDir(dir string) { c.DirV = dir }

func (c *Cmd) Env() []string { return c.EnvV }

func (c *Cmd) SetEnv(env []string) { c.EnvV = env }

func (c *Cmd) Stdin() io.Reader { return c.StdinV }

func (c *Cmd) SetStdin(r io.Reader) { c.StdinV = r }

func (c *Cmd) Stdout() io.Writer { return c.StdoutV }

func (c *Cmd) SetStdout(w io.Writer) { c.StdoutV = w }

func (c *Cmd) Stderr() io.Writer { return c.StderrV }

func (c *Cmd) SetStderr(w io.Writer) { c.StderrV = w }

func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	return nil, errors.New("ocitest: StdoutPipe is not supported")
}

func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	return nil, errors.New("ocitest: StderrPipe is not supported")
}

func (c *Cmd) Run() error {
	c.Ran = true
	if c.RunFunc != nil {
		return c.RunFunc(c)
	}
	return nil
}

func (c *Cmd) Start() error {
	c.Started = true
	if c.StartFunc != nil {
		return c.StartFunc(c)
	}
	return nil
}

func (c *Cmd) Wait() error {
	c.Waited = true
	if c.WaitFunc != nil {
		return c.WaitFunc(c)
	}
	return nil
}

func (c *Cmd) Output() ([]byte, error) {
	if c.OutputFunc != nil {
		return c.OutputFunc(c)
	}
	return nil, nil
}

func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.CombinedOutputFunc != nil {
		return c.CombinedOutputFunc(c)
	}
	return nil, nil
}

func (c *Cmd) Process() *os.Process { return nil }

func (c *Cmd) Err() error { return c.ErrV }

func (c *Cmd) SetWaitDelay(d time.Duration) { c.WaitDelayV = d }

func (c *Cmd) SetCancel(fn func() error) { c.CancelFunc = fn }
