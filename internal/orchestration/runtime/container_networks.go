package runtime

import "context"

// ContainerNetworkState describes an engine-managed network.
type ContainerNetworkState struct {
	// ID is the engine's resource identifier.
	ID string
	// Name is the configured network name.
	Name string
	// Containers lists names of currently attached containers.
	Containers []string
}

// ContainerNetworks manages networks and container attachments without composing services.
type ContainerNetworks interface {
	// EnsureNetwork creates a missing network or reuses an existing one.
	EnsureNetwork(context.Context, string) error
	InspectNetwork(context.Context, string) (ContainerNetworkState, error)
	// RemoveNetwork removes a network. Absence is a no-op. Attached resources remain protected.
	RemoveNetwork(context.Context, string) error
	// SetNetworkAliases sets additional names on an existing network, attaching
	// the container if necessary. Changes may briefly disconnect the container.
	// Failed updates attempt to restore the previous attachment independently
	// of caller cancellation. Callers must handle partial failure.
	SetNetworkAliases(context.Context, string, string, []string) error
}
