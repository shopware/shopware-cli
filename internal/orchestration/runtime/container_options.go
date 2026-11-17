package runtime

// MountType identifies the source of a container mount.
type MountType string

const (
	// MountBind mounts a host path. It is also the default mount type.
	MountBind MountType = "bind"
	// MountVolume mounts a named engine-managed volume.
	MountVolume MountType = "volume"
)

// ContainerMount mounts a host path or named volume inside a container.
type ContainerMount struct {
	// Type selects a bind mount or named volume. Empty means MountBind.
	Type MountType
	// Source is an absolute host path for bind mounts or a volume name.
	Source string
	// Target is an absolute path inside the container.
	Target string
	// ReadOnly prevents the container from modifying the mounted path.
	ReadOnly bool
}

// ContainerPort publishes a container port on the host.
type ContainerPort struct {
	// HostIP restricts the host binding. Empty uses the engine default.
	HostIP string
	// HostPort selects the published port. Zero requests an available port.
	HostPort uint16
	// ContainerPort is the port inside the container.
	ContainerPort uint16
	// Protocol is tcp or udp. Empty means tcp.
	Protocol string
}

// ContainerNetwork attaches a container to a named network.
type ContainerNetwork struct {
	// Name identifies an existing network.
	Name string
	// Aliases supplies additional names for this container on the network.
	Aliases []string
}

// CreateContainerOptions describes a persistent container without starting it.
// The image entrypoint is retained unless Entrypoint is set.
type CreateContainerOptions struct {
	// Name identifies the resource across invocations.
	Name string
	// Image selects the container image.
	Image string
	// Entrypoint optionally replaces the image entrypoint.
	Entrypoint string
	// Args supplies arguments to the selected entrypoint.
	Args []string
	// Dir is the working directory inside the container.
	Dir string
	// User selects the container user. Empty uses the image default.
	User string
	// Env overrides image environment values.
	Env map[string]string
	// Mounts binds host paths or named volumes into the container.
	Mounts []ContainerMount
	// Labels records ownership and configuration metadata.
	Labels map[string]string
	// Ports publishes service endpoints.
	Ports []ContainerPort
	// Networks attaches the container to existing networks.
	Networks []ContainerNetwork
	// RestartPolicy is no, always, unless-stopped or on-failure. Empty means no.
	RestartPolicy string
}
