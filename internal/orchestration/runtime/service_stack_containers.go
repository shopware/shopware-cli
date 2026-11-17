package runtime

import "context"

// ServiceStackContainers identifies containers belonging to a service stack.
// IDs are used with the same engine that backs the stack.
type ServiceStackContainers interface {
	// Containers returns existing service containers, including stopped ones.
	// Missing services have no entries. Scaled services can have several entries.
	Containers(context.Context) ([]ServiceContainer, error)
}

// ServiceContainer maps a logical service to an engine container.
type ServiceContainer struct {
	Service string // Logical service name, such as web.
	ID      string // Container ID understood by the stack's engine.
}
