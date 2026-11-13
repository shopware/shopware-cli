package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

type process struct {
	cmd      *exec.Cmd
	done     chan struct{}
	inputErr <-chan error
	outputs  *outputWrites

	// mu coordinates completion publication with Stop's signal decision.
	mu     sync.Mutex
	result runtime.Result
	err    error
}

// Wait returns the published result after the process and its output copies finish.
func (p *process) Wait() (runtime.Result, error) {
	<-p.done
	result := p.result
	if result.Usage != nil {
		usage := *result.Usage
		result.Usage = &usage
	}
	return result, p.err
}

// reap collects the process result and available stream errors for all waiters.
func (p *process) reap(ctx context.Context) {
	err := p.cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()
	// Closing done publishes the result to every waiter.
	defer close(p.done)

	p.result.ExitCode = -1
	if p.cmd.ProcessState != nil {
		p.result.ExitCode = p.cmd.ProcessState.ExitCode()
		p.result.Usage = &runtime.ResourceUsage{
			CPUTime: p.cmd.ProcessState.UserTime() + p.cmd.ProcessState.SystemTime(),
		}
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			p.err = &runtime.ExitError{Code: p.result.ExitCode}
		} else {
			p.err = &runtime.ExecutionError{Stage: runtime.StageWait, Err: err}
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		cancellation := &runtime.ExecutionError{Stage: runtime.StageWait, Err: ctxErr}
		var exitErr *runtime.ExitError
		if p.err == nil || (p.result.ExitCode < 0 && errors.As(p.err, &exitErr)) {
			p.err = cancellation
		} else {
			p.err = errors.Join(p.err, cancellation)
		}
	}

	// Cmd.Wait can prefer an exit failure over an output copy failure.
	p.outputs.mu.Lock()
	outputErr := p.outputs.err
	p.outputs.mu.Unlock()
	if outputErr != nil && !errors.Is(p.err, outputErr) {
		outputErr = &runtime.ExecutionError{Stage: runtime.StageWait, Err: outputErr}
		if p.err == nil {
			p.err = outputErr
		} else {
			p.err = errors.Join(p.err, outputErr)
		}
	}

	// A reader may still be blocked, so only collect an already completed copy.
	select {
	case inputErr := <-p.inputErr:
		if inputErr != nil && !errors.Is(inputErr, syscall.EPIPE) {
			inputErr = &runtime.ExecutionError{Stage: runtime.StageWait, Err: fmt.Errorf("cannot read process input: %w", inputErr)}
			if p.err == nil {
				p.err = inputErr
			} else {
				p.err = errors.Join(p.err, inputErr)
			}
		}
	default:
	}
}

// Stop requests termination and uses ctx to bound the wait for completion.
func (p *process) Stop(ctx context.Context) error {
	p.mu.Lock()
	select {
	case <-p.done:
		p.mu.Unlock()
		return nil
	default:
	}
	err := killProcess(p.cmd)
	p.mu.Unlock()

	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return &runtime.ExecutionError{Stage: runtime.StageStop, Err: err}
	}

	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return &runtime.ExecutionError{Stage: runtime.StageStop, Err: ctx.Err()}
	}
}
