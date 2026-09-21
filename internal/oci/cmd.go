package oci

import (
	"io"
	"os"
	"os/exec"
	"time"
)

// Cmd is a single external command invocation. It mirrors the subset of
// os/exec.Cmd the CLI relies on, so commands produced by a Runtime (or by the
// project executors) can be faked in memory by tests instead of stubbing
// binaries on PATH.
type Cmd interface {
	// Args returns the command line, binary first.
	Args() []string

	// Dir reports the working directory; SetDir changes it ("" = current dir).
	Dir() string
	SetDir(dir string)

	// Env reports the process environment (nil = inherit); SetEnv replaces it.
	Env() []string
	SetEnv(env []string)

	// Stdin/Stdout/Stderr report the wired streams; the setters rewire them.
	Stdin() io.Reader
	SetStdin(r io.Reader)
	Stdout() io.Writer
	SetStdout(w io.Writer)
	Stderr() io.Writer
	SetStderr(w io.Writer)

	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)

	Run() error
	Start() error
	Wait() error
	Output() ([]byte, error)
	CombinedOutput() ([]byte, error)

	// Process returns the OS process once the command was started, nil before.
	Process() *os.Process

	// Err reports a command construction error (e.g. unresolvable binary)
	// that would surface when the command is run.
	Err() error

	// SetWaitDelay and SetCancel mirror the exec.Cmd knobs governing how a
	// context-cancelled command shuts down.
	SetWaitDelay(d time.Duration)
	SetCancel(fn func() error)
}

// execCmd adapts *exec.Cmd to Cmd.
type execCmd struct {
	cmd *exec.Cmd
}

// WrapCommand adapts an *exec.Cmd built outside a Runtime (e.g. a local PHP
// or ssh invocation) to the Cmd interface.
func WrapCommand(cmd *exec.Cmd) Cmd {
	return &execCmd{cmd: cmd}
}

func (c *execCmd) Args() []string { return c.cmd.Args }

func (c *execCmd) Dir() string { return c.cmd.Dir }

func (c *execCmd) SetDir(dir string) { c.cmd.Dir = dir }

func (c *execCmd) Env() []string { return c.cmd.Env }

func (c *execCmd) SetEnv(env []string) { c.cmd.Env = env }

func (c *execCmd) Stdin() io.Reader { return c.cmd.Stdin }

func (c *execCmd) SetStdin(r io.Reader) { c.cmd.Stdin = r }

func (c *execCmd) Stdout() io.Writer { return c.cmd.Stdout }

func (c *execCmd) SetStdout(w io.Writer) { c.cmd.Stdout = w }

func (c *execCmd) Stderr() io.Writer { return c.cmd.Stderr }

func (c *execCmd) SetStderr(w io.Writer) { c.cmd.Stderr = w }

func (c *execCmd) StdoutPipe() (io.ReadCloser, error) { return c.cmd.StdoutPipe() }

func (c *execCmd) StderrPipe() (io.ReadCloser, error) { return c.cmd.StderrPipe() }

func (c *execCmd) Run() error { return c.cmd.Run() }

func (c *execCmd) Start() error { return c.cmd.Start() }

func (c *execCmd) Wait() error { return c.cmd.Wait() }

func (c *execCmd) Output() ([]byte, error) { return c.cmd.Output() }

func (c *execCmd) CombinedOutput() ([]byte, error) { return c.cmd.CombinedOutput() }

func (c *execCmd) Process() *os.Process { return c.cmd.Process }

func (c *execCmd) Err() error { return c.cmd.Err }

func (c *execCmd) SetWaitDelay(d time.Duration) { c.cmd.WaitDelay = d }

func (c *execCmd) SetCancel(fn func() error) { c.cmd.Cancel = fn }
