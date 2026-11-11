// Package container binds container runners to the runtime contract.
package container

import (
	"context"
	"errors"

	containerapi "github.com/shopware/shopware-cli/internal/orchestration/container"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var errNilRunner = errors.New("container runner is nil")

// Backend runs commands through a container runner with bound options.
type Backend struct {
	runner  containerapi.Runner
	options containerapi.RunOptions
}

// New binds a container runner and its options as a runtime backend.
func New(runner containerapi.Runner, options containerapi.RunOptions) *Backend {
	return &Backend{runner: runner, options: cloneRunOptions(options)}
}

// Start delegates the command and streams to the container runner.
func (b *Backend) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if b.runner == nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errNilRunner}
	}
	return b.runner.Run(ctx, cloneRunOptions(b.options), command, options)
}

func cloneRunOptions(options containerapi.RunOptions) containerapi.RunOptions {
	options.Mounts = append([]containerapi.Mount(nil), options.Mounts...)
	return options
}
