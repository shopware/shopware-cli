package container

import (
	"context"
	"errors"
)

// ErrNotFound reports an absent managed resource, not an unavailable engine.
var ErrNotFound = errors.New("container resource not found")

// Port publishes a container port on the host.
type Port struct {
	// HostIP restricts the host binding. Empty uses the engine default.
	HostIP string
	// HostPort selects the published port. Zero requests an available port.
	HostPort uint16
	// ContainerPort is the port inside the container.
	ContainerPort uint16
	// Protocol is tcp or udp. Empty means tcp.
	Protocol string
}

// CreateOptions describes a persistent container, retaining the image entrypoint
// unless Entrypoint is set. Its lifetime is independent of a management call.
type CreateOptions struct {
	// Name identifies the resource across invocations.
	Name string
	// Image selects the container image.
	Image string
	// Entrypoint optionally replaces the image entrypoint.
	Entrypoint string
	// Args supplies arguments to the selected entrypoint.
	Args []string
	// Mounts binds files or directories into the container.
	Mounts []Mount
	// Labels records ownership and configuration metadata.
	Labels map[string]string
	// Ports publishes service endpoints.
	Ports []Port
	// Network attaches the container to an existing network.
	Network string
	// RestartPolicy selects the restart behavior, such as unless-stopped.
	RestartPolicy string
}

// State describes a managed container independently of a CLI process handle.
type State struct {
	// ID is the engine's resource identifier.
	ID string
	// Status is the engine-reported lifecycle state.
	Status string
	// Running reports whether the container is running.
	Running bool
	// RestartCount counts engine-managed restarts.
	RestartCount int
	// Labels contains the current resource metadata.
	Labels map[string]string
}

// Lifecycle manages persistent containers. Successful calls do not attach
// resource lifetime to their contexts. Removal is always explicit.
type Lifecycle interface {
	Inspect(context.Context, string) (State, error)
	Create(context.Context, CreateOptions) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	// Remove also terminates a running container. An absent container is a no-op.
	Remove(context.Context, string) error
}

// Networks provisions named networks independently of container execution.
type Networks interface {
	EnsureNetwork(context.Context, string) error
	RemoveNetwork(context.Context, string) error
}
