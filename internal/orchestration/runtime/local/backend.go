// Package local runs commands directly on the host operating system.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Backend executes commands on the host.
type Backend struct{}

// New returns a host process backend.
func New() *Backend {
	return &Backend{}
}

// Start starts a command directly, without a shell.
// Bare executable names use the CLI's PATH for lookup. Env overlays affect the
// child environment; use an absolute name to select a tool outside that PATH.
func (*Backend) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if options.TTY {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: runtime.ErrUnsupported}
	}
	if command.Name == "" {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("command name is empty")}
	}
	if err := ctx.Err(); err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: err}
	}

	cmd := exec.Command(command.Name, command.Args...) //nolint:noctx // Context cancellation calls Stop to terminate the process group.
	cmd.Dir = command.Dir
	cmd.Env = overlayEnv(os.Environ(), command.Env)

	var stdin io.WriteCloser
	if options.Stdin != nil {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot create standard input pipe: %w", err)}
		}
	}

	outputs := &outputWrites{}
	cmd.Stdout = trackedWriter{writer: writerOrDiscard(options.Stdout), state: outputs}
	cmd.Stderr = trackedWriter{writer: writerOrDiscard(options.Stderr), state: outputs}
	configureProcess(cmd)

	if err := cmd.Start(); err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("cannot start %q: %w", command.Name, err)}
	}

	// Do not let a caller-owned reader block Wait after the process exits.
	// The caller must unblock a pending read when it no longer needs the input.
	var inputErr <-chan error
	if stdin != nil {
		completed := make(chan error, 1)
		inputErr = completed
		go func() {
			_, err := io.Copy(stdin, options.Stdin)
			// Publish the copy error before the process can observe EOF.
			completed <- err
			_ = stdin.Close()
		}()
	}

	proc := &process{
		cmd:      cmd,
		done:     make(chan struct{}),
		inputErr: inputErr,
		outputs:  outputs,
	}
	go proc.reap(ctx)
	go func() {
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = proc.Stop(stopCtx)
		case <-proc.done:
		}
	}()

	return proc, nil
}

func overlayEnv(base []string, overlay map[string]string) []string {
	values := make(map[string]string, len(base)+len(overlay))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	maps.Copy(values, overlay)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
