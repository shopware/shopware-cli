package runtime

import (
	"context"
	"io"
)

// Containers groups persistent-container capabilities offered by an engine.
// Consumers can declare smaller interfaces for the operations they need.
// Resources outlive operation contexts. Creation never starts or replaces them.
type Containers interface {
	// ListContainers returns summaries, including stopped containers.
	ListContainers(context.Context, ListContainersOptions) ([]ContainerSummary, error)
	// InspectContainer returns details. Verified absence returns ErrNotFound.
	InspectContainer(context.Context, string) (ContainerState, error)
	// CreateContainer creates without starting or replacing a container.
	CreateContainer(context.Context, CreateContainerOptions) (string, error)
	// StartContainer starts an existing container.
	StartContainer(context.Context, string) error
	// StopContainer stops a container while retaining its resources.
	StopContainer(context.Context, string) error
	// RestartContainer restarts an existing container.
	RestartContainer(context.Context, string) error
	// RemoveContainer terminates and removes a container, retaining named volumes.
	// Missing containers are ignored.
	RemoveContainer(context.Context, string) error
	// Logs forwards stdout and stderr separately. Nil writers discard their stream.
	// Follow waits for more until canceled, without stopping the container.
	Logs(ctx context.Context, id string, options ContainerLogOptions) error
}

// ListContainersOptions restricts a container listing.
type ListContainersOptions struct {
	// IDs selects exact container IDs. Empty selects all containers.
	// Missing IDs are omitted.
	IDs []string
}

// ContainerSummary provides the fields needed for a container listing.
type ContainerSummary struct {
	ID     string          // Engine-assigned container ID.
	Name   string          // Container name.
	Image  string          // Image reference reported by the engine.
	Status ContainerStatus // Current normalized container state.
	Ports  []ContainerPort // Published ports with resolved host bindings.
}

// ContainerLogOptions selects historical output, following and output writers.
type ContainerLogOptions struct {
	Follow bool      // Continue reading new output until canceled.
	Tail   int       // Last N lines before following. Zero means all available lines.
	Stdout io.Writer // Receives stdout. Nil discards it.
	Stderr io.Writer // Receives stderr. Nil discards it.
}
