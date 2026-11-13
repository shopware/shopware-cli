// Package stack defines explicit service and lifecycle boundaries.
package stack

import (
	"context"
	"errors"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Lifecycle starts and stops a stack independently of service execution.
// Results describe the lifecycle operation, not a service workload.
type Lifecycle interface {
	// Start brings up services; readiness is provider-specific.
	Start(context.Context, runtime.Options) (runtime.Result, error)
	// Stop stops services while retaining stack resources.
	Stop(context.Context, runtime.Options) (runtime.Result, error)
}

// ServiceExecutor starts commands in a named service.
type ServiceExecutor interface {
	Exec(context.Context, string, runtime.Command, runtime.Options) (runtime.Process, error)
}

// RemoveOptions controls which resources are removed with a stack.
type RemoveOptions struct {
	// RemoveVolumes also removes named volumes.
	RemoveVolumes bool
}

// Remover removes stack resources.
type Remover interface {
	Remove(context.Context, RemoveOptions, runtime.Options) (runtime.Result, error)
}

type serviceBackend struct {
	executor ServiceExecutor
	service  string
}

// Service binds an executor to a service without changing stack lifecycle.
func Service(executor ServiceExecutor, service string) runtime.Backend {
	return serviceBackend{executor: executor, service: service}
}

func (s serviceBackend) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if s.executor == nil || s.service == "" {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("service and executor are required")}
	}
	return s.executor.Exec(ctx, s.service, command, options)
}
