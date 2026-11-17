package runtime

// ContainerStatus is the common lifecycle state reported by container engines.
type ContainerStatus string

const (
	// ContainerUnknown is the zero value and covers unrecognized engine states.
	ContainerUnknown ContainerStatus = ""
	// ContainerCreated identifies a container that has not been started.
	ContainerCreated ContainerStatus = "created"
	// ContainerRunning identifies an active container.
	ContainerRunning ContainerStatus = "running"
	// ContainerStopped identifies a container whose main process has exited.
	ContainerStopped ContainerStatus = "stopped"
	// ContainerPaused identifies a suspended container.
	ContainerPaused ContainerStatus = "paused"
	// ContainerRestarting identifies a container being restarted by its engine.
	ContainerRestarting ContainerStatus = "restarting"
	// ContainerFailed identifies an unusable container requiring intervention.
	ContainerFailed ContainerStatus = "failed"
)

// ContainerState describes a persistent container independently of a process.
type ContainerState struct {
	// ID is the engine's resource identifier.
	ID string
	// Name is the container's configured name.
	Name string
	// Image is the image reference reported by the engine.
	Image string
	// Ports contains published ports with resolved host bindings.
	Ports []ContainerPort
	// Status is the normalized lifecycle state, or ContainerUnknown.
	Status ContainerStatus
	// Running reports whether the container is running.
	Running bool
	// RestartCount counts engine-managed restarts.
	RestartCount int
	// Labels contains the current resource metadata.
	Labels map[string]string
	// Networks contains the container's current attachments and aliases.
	Networks []ContainerNetwork
}
