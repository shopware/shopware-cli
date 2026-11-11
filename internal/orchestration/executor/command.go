package executor

import (
	"context"
	"fmt"
	"maps"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Command is an immutable command description.
type Command struct {
	executor *Executor
	spec     runtime.Command
	capture  runtime.CaptureMode
}

// Description returns an independent copy of the portable command description.
func (c *Command) Description() runtime.Command {
	return c.clone().spec
}

// WithDir returns a copy configured to run in dir.
func (c *Command) WithDir(dir string) *Command {
	command := c.clone()
	command.spec.Dir = dir
	return command
}

// WithEnv returns a copy with the provided environment values overlaid.
func (c *Command) WithEnv(env map[string]string) *Command {
	command := c.clone()
	command.spec.Env = make(map[string]string, len(c.spec.Env)+len(env))
	maps.Copy(command.spec.Env, c.spec.Env)
	maps.Copy(command.spec.Env, env)

	return command
}

// WithCapture returns a copy that retains output in the selected mode.
// Captured output is unbounded until the process completes; callers control its lifetime.
func (c *Command) WithCapture(mode runtime.CaptureMode) *Command {
	command := c.clone()
	command.capture = mode
	return command
}

// clone isolates the mutable argument and environment data.
func (c *Command) clone() *Command {
	spec := c.spec
	spec.Args = append([]string(nil), c.spec.Args...)
	spec.Env = maps.Clone(c.spec.Env)
	return &Command{executor: c.executor, spec: spec, capture: c.capture}
}

// Run starts the command and waits for it to finish. The result retains
// available exit and captured output data even when an error is returned.
func (c *Command) Run(ctx context.Context, options runtime.Options) (runtime.Result, error) {
	process, err := c.Start(ctx, options)
	if err != nil {
		return runtime.Result{ExitCode: -1, Output: runtime.CapturedOutput{Mode: c.capture}}, err
	}

	return process.Wait()
}

// Start starts the command and returns its process handle.
func (c *Command) Start(ctx context.Context, options runtime.Options) (runtime.Process, error) {
	switch c.capture {
	case runtime.CaptureNone:
		return c.executor.backend.Start(ctx, c.clone().spec, options)
	case runtime.CaptureSeparate, runtime.CaptureCombined:
	default:
		return nil, &runtime.ExecutionError{
			Stage: runtime.StageStart,
			Err:   fmt.Errorf("invalid capture mode %d", c.capture),
		}
	}

	capture := &outputCapture{mode: c.capture}
	options.Stdout = captureWriter{
		capture: capture,
		target:  options.Stdout,
	}
	options.Stderr = captureWriter{
		capture: capture,
		target:  options.Stderr,
		stderr:  true,
	}

	process, err := c.executor.backend.Start(ctx, c.clone().spec, options)
	if err != nil {
		return nil, err
	}

	return &capturedProcess{
		process: process,
		capture: capture,
	}, nil
}

// Capture runs the command and retains stdout and stderr separately.
func (c *Command) Capture(ctx context.Context) (runtime.Result, error) {
	return c.WithCapture(runtime.CaptureSeparate).Run(ctx, runtime.Options{})
}

// CombinedOutput runs the command and retains both output streams in observed write order.
func (c *Command) CombinedOutput(ctx context.Context) (runtime.Result, error) {
	return c.WithCapture(runtime.CaptureCombined).Run(ctx, runtime.Options{})
}
