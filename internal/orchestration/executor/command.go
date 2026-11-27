package executor

import (
	"context"
	"maps"
	"slices"
	"sync/atomic"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
	"github.com/shopware/shopware-cli/logging"
)

// nextExecutionID correlates concurrent attempts, including repeated commands.
var nextExecutionID atomic.Uint64

// Command describes a reusable invocation. Derived settings leave the original
// unchanged. Methods may be called concurrently.
type Command struct {
	target runtime.Runtime
	spec   runtime.Command
}

// Description returns an independent copy of the command specification.
// Its environment can contain credentials, so diagnostics must not log Env.
func (c *Command) Description() runtime.Command {
	description := c.spec
	description.Args = slices.Clone(description.Args)
	description.Env = maps.Clone(description.Env)
	return description
}

// WithDir returns a copy with the target working directory replaced.
func (c *Command) WithDir(dir string) *Command {
	derived := *c
	derived.spec.Dir = dir
	return &derived
}

// WithEnv returns a copy with env overlaid on the command's target environment.
func (c *Command) WithEnv(env map[string]string) *Command {
	derived := *c
	derived.spec.Env = overlayEnv(c.spec.Env, env)
	return &derived
}

// Start starts the command. The context controls execution lifetime, including
// after Start returns. Capture is implemented using the runtime's output writers.
// A successful start returns a process whose Wait method observes completion.
// It does not establish that the workload will exit successfully.
// The runtime's own log events carry the execution ID for correlation.
func (c *Command) Start(ctx context.Context, options runtime.Options) (_ runtime.Process, startErr error) {
	logger := logging.FromContext(ctx).With("execution_id", nextExecutionID.Add(1))
	ctx = logging.WithLogger(ctx, logger)

	defer func() {
		if startErr != nil {
			logger.Debugw("Cannot start command", "event", "executor_start_failed", "executable", c.spec.Name)
		}
	}()

	options, output, err := captureOptions(options)
	if err != nil {
		return nil, err
	}

	started, err := c.target.Start(ctx, c.Description(), options)
	if err != nil {
		return nil, err
	}

	return &process{Process: started, capture: output}, nil
}

// process adds captured output to the runtime's result and turns an
// unsuccessful exit into a *runtime.ExitError. Runtimes report exits only
// through Result.ExitCode. Termination requests go unchanged to the runtime.
type process struct {
	runtime.Process
	capture *capture // Nil without capture.
}

// Wait returns the runtime's failure, joined with ErrCaptureLimit when output
// was cut off, or an ExitError when the workload exited unsuccessfully without
// either. Each caller owns its output.
func (p *process) Wait() (runtime.Result, error) {
	result, err := p.Process.Wait()
	if p.capture != nil {
		var limitErr error
		if result.Output, limitErr = p.capture.snapshot(); limitErr != nil {
			err = runtime.JoinErrors(err, limitErr)
		}
	}

	if err == nil && result.ExitCode != 0 {
		err = &runtime.ExitError{Code: result.ExitCode, Signal: result.Signal}
	}

	return result, err
}

// Run starts the command and waits for exit, output forwarding and cleanup.
// An error is either a *runtime.ExitError for an unsuccessful exit or a runtime
// failure, never both. Results and captured output remain available on failure.
// A startup failure has exit code -1.
func (c *Command) Run(ctx context.Context, options runtime.Options) (runtime.Result, error) {
	process, err := c.Start(ctx, options)
	if err != nil {
		return runtime.Result{ExitCode: -1, Output: runtime.CapturedOutput{Mode: options.Capture}}, err
	}

	return process.Wait()
}
