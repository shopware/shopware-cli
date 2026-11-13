// Package docker runs commands in disposable containers through the Docker CLI.
package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/orchestration/container"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime/local"
)

const cleanupTimeout = 30 * time.Second

// Client starts Docker CLI commands using the local process backend.
type Client struct {
	backend runtime.Backend
}

// New creates a Docker CLI client backed by local process execution.
func New() *Client {
	return NewWithBackend(local.New())
}

// NewWithBackend creates a Docker CLI client with an injectable process backend.
func NewWithBackend(backend runtime.Backend) *Client {
	return &Client{backend: backend}
}

// Run starts a disposable container for command and forwards its streams.
func (c *Client) Run(ctx context.Context, options container.RunOptions, command runtime.Command, streams runtime.Options) (runtime.Process, error) {
	if err := validate(options, command, streams); err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: err}
	}
	if c.backend == nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("docker process backend is nil")}
	}

	name, err := newContainerName()
	if err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot create container name: %w", err)}
	}

	dockerCommand := runtime.Command{Name: "docker", Args: runArgs(name, options, command, streams)}
	client, err := c.backend.Start(ctx, dockerCommand, streams)
	if err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot start Docker CLI: %w", err)}
	}

	proc := &process{
		backend:     c.backend,
		client:      client,
		name:        name,
		cleanupDone: make(chan struct{}),
		waitDone:    make(chan struct{}),
	}
	go proc.finalize(ctx)

	// Cancellation can happen before Wait is called, so cleanup is monitored
	// independently of the caller waiting for the Docker CLI process.
	go func() {
		select {
		case <-ctx.Done():
			_ = proc.Stop(context.Background())
		case <-proc.cleanupDone:
		}
	}()

	return proc, nil
}

func validate(options container.RunOptions, command runtime.Command, streams runtime.Options) error {
	if streams.TTY {
		return fmt.Errorf("%w: Docker TTY mode is not supported", runtime.ErrUnsupported)
	}
	if options.Image == "" || strings.TrimSpace(options.Image) != options.Image || strings.HasPrefix(options.Image, "-") || hasControl(options.Image) {
		return errors.New("invalid container image")
	}
	if command.Name == "" || hasControl(command.Name) {
		return errors.New("container command name must not be empty or contain control characters")
	}
	if strings.HasPrefix(command.Name, "-") {
		return errors.New("container command name must not start with a hyphen")
	}
	if options.User != "" && hasControl(options.User) {
		return errors.New("invalid container user")
	}

	for _, mount := range options.Mounts {
		if !filepath.IsAbs(mount.Source) || !filepath.IsAbs(mount.Target) {
			return errors.New("container mount source and target must be absolute paths")
		}
		if hasControl(mount.Source) || hasControl(mount.Target) || strings.ContainsAny(mount.Source+mount.Target, ",") {
			return errors.New("container mount paths cannot contain commas or control characters")
		}
	}

	for key := range command.Env {
		if !validEnvKey(key) {
			return fmt.Errorf("invalid container environment key %q", key)
		}
	}

	return nil
}

func runArgs(name string, options container.RunOptions, command runtime.Command, streams runtime.Options) []string {
	args := []string{"run", "--rm", "--name", name}
	for _, mount := range options.Mounts {
		value := "type=bind,src=" + mount.Source + ",dst=" + mount.Target
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}

	if options.User != "" {
		args = append(args, "--user", options.User)
	}
	if command.Dir != "" {
		args = append(args, "--workdir", command.Dir)
	}

	envKeys := make([]string, 0, len(command.Env))
	for key := range command.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		args = append(args, "--env", key+"="+command.Env[key])
	}

	if streams.Stdin != nil {
		args = append(args, "-i")
	}

	args = append(args, "--entrypoint", command.Name, options.Image)
	return append(args, command.Args...)
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}

	for index, char := range key {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}

	return true
}

func hasControl(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}

func newContainerName() (string, error) {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}

	return "shopware-cli-" + hex.EncodeToString(suffix[:]), nil
}

// process coordinates the Docker CLI process with the disposable container.
// Wait and Stop may be called concurrently and share their results across calls.
type process struct {
	backend runtime.Backend
	client  runtime.Process
	name    string

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
	p.startCleanup(true)

	// Prefer the published finalization result when it is already available.
	select {
	case <-p.waitDone:
		return p.stopErr
	default:
	}
	select {
	case <-p.waitDone:
		return p.stopErr
	case <-ctx.Done():
		return &runtime.ExecutionError{Stage: runtime.StageStop, Err: ctx.Err()}
	}
}

// startCleanup runs once independently of each caller's wait context.
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
		if err := p.client.Stop(stopCtx); err != nil {
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
	client, err := p.backend.Start(ctx, runtime.Command{Name: "docker", Args: []string{"rm", "--force", p.name}}, runtime.Options{Stderr: &stderr})
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

// A Docker CLI exit describes the supporting process, not the workload. Keep
// other error identities available without exposing that exit as a workload exit.
func hideClientExit(err error) error {
	var exitErr *runtime.ExitError
	if errors.As(err, &exitErr) {
		return clientError{err: err}
	}
	return err
}

type clientError struct{ err error }

func (e clientError) Error() string { return e.err.Error() }

func (e clientError) Is(target error) bool {
	if _, isExit := target.(*runtime.ExitError); isExit {
		return false
	}
	return errors.Is(e.err, target)
}

func (e clientError) As(target any) bool {
	if _, isExit := target.(**runtime.ExitError); isExit {
		return false
	}
	return errors.As(e.err, target)
}
