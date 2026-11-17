// Package docker implements command execution and container resource management.
package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Options configures disposable commands run by a Docker runtime.
type Options struct {
	// Image identifies the container image to run.
	Image string
	// Mounts binds host paths into the container.
	Mounts []runtime.ContainerMount
	// User selects the container user. An empty value uses the image default.
	User string
}

// Runtime manages containers and starts disposable commands through the Docker CLI.
type Runtime struct {
	host    runtime.Runtime
	options Options
}

// New binds disposable-container options to the host command runtime.
func New(host runtime.Runtime, options Options) *Runtime {
	options.Mounts = append([]runtime.ContainerMount(nil), options.Mounts...)
	return &Runtime{host: host, options: options}
}

// Start starts a disposable container for command and forwards its streams.
func (c *Runtime) Start(ctx context.Context, command runtime.Command, streams runtime.Options) (runtime.Process, error) {
	options := c.options
	if err := validate(options, command, streams); err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: err}
	}
	if c.host == nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("docker command runtime is required")}
	}

	name, err := newContainerName()
	if err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot create container name: %w", err)}
	}

	dockerCommand := runtime.Command{Name: "run", Args: runArgs(name, options, command, streams)[1:]}
	client, err := c.ExecDocker(ctx, dockerCommand, streams)
	if err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot start Docker CLI: %w", err)}
	}

	proc := &process{
		docker:      c,
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
			killCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			_ = proc.Kill(killCtx)
			cancel()
			proc.startCleanup(true)
		case <-proc.cleanupDone:
		}
	}()

	return proc, nil
}

func validate(options Options, command runtime.Command, streams runtime.Options) error {
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
		if err := validateMount(mount); err != nil {
			return err
		}
	}

	for key := range command.Env {
		if !validEnvKey(key) {
			return fmt.Errorf("invalid container environment key %q", key)
		}
	}

	return nil
}

func runArgs(name string, options Options, command runtime.Command, streams runtime.Options) []string {
	args := []string{"run", "--rm", "--name", name}
	for _, mount := range options.Mounts {
		value := mountValue(mount)
		args = append(args, "--mount", value)
	}

	if options.User != "" {
		args = append(args, "--user", options.User)
	}
	if command.Dir != "" {
		args = append(args, "--workdir", command.Dir)
	}

	envKeys := slices.Sorted(maps.Keys(command.Env))
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
