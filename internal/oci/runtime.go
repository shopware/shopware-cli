// Package oci centralizes how shopware-cli shells out to an OCI container
// runtime. Every docker invocation in the codebase goes through the Runtime
// interface, so tests can inject a fake via WithRuntime and alternative
// backends (e.g. Podman) only need to implement Runtime to be pluggable.
package oci

import (
	"context"
	"os/exec"
)

// Runtime abstracts the container runtime CLI shopware-cli shells out to.
type Runtime interface {
	// Binary is the runtime executable resolved on PATH (e.g. "docker").
	Binary() string
	// Command returns a command running `<binary> <args...>`, bound to ctx.
	// The caller wires up Dir, Stdout, Stderr, and friends as needed.
	Command(ctx context.Context, args ...string) *exec.Cmd
	// ComposeCommand returns a command running `<binary> compose <args...>`.
	// Compose is a separate seam because other runtimes may not ship it as a
	// CLI plugin subcommand.
	ComposeCommand(ctx context.Context, args ...string) *exec.Cmd
}

// DockerRuntime shells out to the docker CLI.
type DockerRuntime struct{}

func (DockerRuntime) Binary() string { return "docker" }

func (r DockerRuntime) Command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, r.Binary(), args...)
}

func (r DockerRuntime) ComposeCommand(ctx context.Context, args ...string) *exec.Cmd {
	return r.Command(ctx, append([]string{"compose"}, args...)...)
}

type runtimeKey struct{}

var defaultRuntime Runtime = DockerRuntime{}

// Default returns the runtime used when the context carries none.
func Default() Runtime { return defaultRuntime }

// WithRuntime returns a context that makes FromContext return r.
func WithRuntime(ctx context.Context, r Runtime) context.Context {
	return context.WithValue(ctx, runtimeKey{}, r)
}

// FromContext returns the runtime attached to ctx, or Default() when none is.
func FromContext(ctx context.Context) Runtime {
	if r, ok := ctx.Value(runtimeKey{}).(Runtime); ok && r != nil {
		return r
	}
	return defaultRuntime
}
