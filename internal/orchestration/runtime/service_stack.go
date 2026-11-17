package runtime

import "context"

// ServiceStackStatus describes the aggregate process state of required services.
// It does not report readiness or detect crashes.
type ServiceStackStatus string

const (
	// ServiceStackUnknown accompanies an error when the state cannot be determined.
	ServiceStackUnknown ServiceStackStatus = ""
	// ServiceStackRunning means all required services are running.
	ServiceStackRunning ServiceStackStatus = "running"
	// ServiceStackStopped means no required services are running.
	ServiceStackStopped ServiceStackStatus = "stopped"
	// ServiceStackPartial means some, but not all, required services are running.
	ServiceStackPartial ServiceStackStatus = "partial"
)

// ServiceStack manages a persistent group of named services.
// Contexts bound management operations, not the lifetime of started resources.
// Failed operations may leave partial changes; callers decide what to clean up.
type ServiceStack interface {
	// Up starts or reconciles services and returns without attaching to them.
	// Success does not guarantee application readiness.
	Up(context.Context) error
	// Status reports the aggregate process state of required services.
	Status(context.Context) (ServiceStackStatus, error)
	// Stop stops services while retaining their containers and other resources.
	Stop(context.Context) error
	// Service binds command execution to a logical service name. It does not
	// start services or check whether their containers are currently running.
	Service(string) (Runtime, error)
}
