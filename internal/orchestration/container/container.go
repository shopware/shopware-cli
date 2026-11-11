// Package container defines the contract for running commands in containers.
package container

import (
	"context"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Mount binds a host directory or file into a container.
type Mount struct {
	// Source is an absolute path on the host.
	Source string
	// Target is an absolute path inside the container.
	Target string
	// ReadOnly prevents the container from modifying the mounted path.
	ReadOnly bool
}

// RunOptions contains the structured settings for one disposable container run.
type RunOptions struct {
	// Image identifies the container image to run.
	Image string
	// Mounts binds host paths into the container.
	Mounts []Mount
	// User selects the container user. An empty value uses the image default.
	User string
}

// Runner starts a command in a container with the supplied settings.
type Runner interface {
	Run(context.Context, RunOptions, runtime.Command, runtime.Options) (runtime.Process, error)
}
