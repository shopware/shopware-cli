package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

const cleanupTimeout = 30 * time.Second

// process coordinates the Docker CLI process with the disposable container.
// Wait and Stop may be called concurrently and share their results across calls.
type process struct {
	docker *Runtime
	client runtime.Process
	name   string

	stopOnce    sync.Once
	waitDone    chan struct{}
	waitResult  runtime.Result
	waitErr     error
	stopErr     error
	cleanupDone chan struct{}
}

func (p *process) finalize(ctx context.Context) {
	defer close(p.waitDone)
	p.waitResult, p.waitErr = p.client.Wait()
	// The Docker CLI process is transport, so its usage is not workload usage.
	p.waitResult.Usage = nil
	// Docker uses these codes for startup failures, but docker run also
	// forwards workload exits. They cannot be distinguished with this API.
	switch {
	case p.waitResult.ExitCode >= 125 && p.waitResult.ExitCode <= 127:
		startupErr := fmt.Errorf("docker could not start the container command (exit code %d)", p.waitResult.ExitCode)
		if p.waitErr != nil {
			if _, isExit := p.waitErr.(*runtime.ExitError); !isExit {
				startupErr = errors.Join(startupErr, hideClientExit(p.waitErr))
			}
		}
		p.waitErr = &runtime.ExecutionError{Stage: runtime.StageStart, Err: startupErr}
		p.waitResult.ExitCode = -1
	case p.waitResult.ExitCode < 0:
		if p.waitErr == nil {
			p.waitErr = errors.New("docker client exit code is unknown")
		}
		p.waitErr = &runtime.ExecutionError{Stage: runtime.StageWait, Err: hideClientExit(p.waitErr)}
	case p.waitErr == nil && p.waitResult.ExitCode > 0:
		p.waitErr = &runtime.ExitError{Code: p.waitResult.ExitCode}
	case p.waitErr != nil:
		if _, isExit := p.waitErr.(*runtime.ExitError); !isExit {
			p.waitErr = &runtime.ExecutionError{Stage: runtime.StageWait, Err: p.waitErr}
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		if p.waitErr == nil {
			p.waitErr = &runtime.ExecutionError{Stage: runtime.StageWait, Err: ctxErr}
		} else if !errors.Is(p.waitErr, ctxErr) {
			p.waitErr = errors.Join(p.waitErr, &runtime.ExecutionError{Stage: runtime.StageWait, Err: ctxErr})
		}
	}

	p.startCleanup(p.waitErr != nil)
	<-p.cleanupDone
	cleanupErr := p.stopErr
	if cleanupErr != nil {
		if p.waitErr == nil {
			p.waitErr = cleanupErr
		} else {
			p.waitErr = errors.Join(p.waitErr, cleanupErr)
		}
	}
}

func (p *process) Wait() (runtime.Result, error) {
	<-p.waitDone
	return p.waitResult, p.waitErr
}

func (p *process) Stop(ctx context.Context) error {
	return p.signal(ctx, "TERM")
}

func (p *process) Kill(ctx context.Context) error {
	return p.signal(ctx, "KILL")
}

// signal waits for Docker to accept the request, not for the workload to exit.
func (p *process) signal(ctx context.Context, signal string) error {
	if err := ctx.Err(); err != nil {
		return &runtime.ExecutionError{Stage: runtime.StageStop, Err: err}
	}
	select {
	case <-p.waitDone:
		return nil
	default:
	}

	_, diagnostic, err := p.docker.command(ctx, "kill", "--signal", signal, p.name)
	if err == nil {
		return nil
	}
	// The container may have exited and been removed while the request ran.
	if ctx.Err() == nil && (strings.EqualFold(diagnostic, "Error response from daemon: No such container: "+p.name) ||
		strings.EqualFold(diagnostic, "Error response from daemon: Cannot kill container: "+p.name+": container is not running")) {
		select {
		case <-p.waitDone:
			return nil
		default:
		}
	}
	return &runtime.ExecutionError{Stage: runtime.StageStop, Err: fmt.Errorf("cannot signal container %s: %w", p.name, err)}
}

// startCleanup runs once independently of termination request contexts.
func (p *process) startCleanup(stopClient bool) {
	p.stopOnce.Do(func() {
		go p.cleanup(stopClient)
	})
}

func (p *process) cleanup(stopClient bool) {
	// Cleanup has its own bounded context because the workload context may
	// already be cancelled. Stop the client before removing the container.
	var clientErr error
	if stopClient {
		stopCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := p.client.Kill(stopCtx); err != nil {
			clientErr = &runtime.ExecutionError{Stage: runtime.StageStop, Err: hideClientExit(err)}
		}
	}
	cleanupErr := p.removeContainer()
	if cleanupErr != nil {
		cleanupErr = &runtime.ExecutionError{Stage: runtime.StageCleanup, Err: cleanupErr}
	}
	p.stopErr = errors.Join(clientErr, cleanupErr)
	// Closing cleanupDone publishes stopErr to all waiters.
	close(p.cleanupDone)
}

func (p *process) removeContainer() error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	var stderr bytes.Buffer
	client, err := p.docker.ExecDocker(ctx, runtime.Command{Name: "rm", Args: []string{"--force", p.name}}, runtime.Options{Stderr: &stderr})
	if err != nil {
		return fmt.Errorf("cannot remove container %s: %w", p.name, hideClientExit(err))
	}

	result, err := client.Wait()
	if err != nil || result.ExitCode != 0 {
		exitErr, directExit := err.(*runtime.ExitError)
		if result.ExitCode > 0 && result.ExitCode < 125 &&
			(err == nil || (directExit && exitErr.Code == result.ExitCode)) &&
			strings.EqualFold(strings.TrimSpace(stderr.String()), "Error response from daemon: No such container: "+p.name) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot remove container %s: %w: %s", p.name, hideClientExit(err), strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("cannot remove container %s: docker rm exited with code %d", p.name, result.ExitCode)
	}

	return nil
}
