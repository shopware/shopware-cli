package docker

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/shop"
)

// PortDefinition describes one host port the generated compose.yaml publishes.
type PortDefinition struct {
	// Key is the docker.ports config key (shop.DockerPort* constant).
	Key string
	// Service is the compose service publishing the port.
	Service string
	// Label is the human-readable name shown in conflict messages.
	Label string
	// Target is the fixed container-side port.
	Target int
	// Default is the host port used when no override is configured.
	Default int
}

// portDefinitions flattens the keyed endpoints of the given services into
// port definitions, in the order the services appear in the generated file.
func portDefinitions(services []ServiceDefinition) []PortDefinition {
	var defs []PortDefinition
	for _, svc := range services {
		for _, ep := range svc.Endpoints {
			if ep.Key == "" {
				continue
			}
			defs = append(defs, PortDefinition{
				Key:     ep.Key,
				Service: svc.Name,
				Label:   ep.Label,
				Target:  ep.ContainerPort,
				Default: ep.DefaultHostPort,
			})
		}
	}

	return defs
}

// HostPort returns the host port configured for key, falling back to the
// catalog default when no override is set. It returns 0 when the port is
// disabled (configured as false) and must not be published.
func HostPort(ports shop.ConfigDockerPorts, key string) int {
	if port, ok := ports[key]; ok {
		if port.Disabled() {
			return 0
		}
		if port > 0 {
			return int(port)
		}
	}

	if ep := EndpointByKey(key); ep != nil {
		return ep.DefaultHostPort
	}

	return 0
}

// PortConflict reports a host port that is already bound by another process.
type PortConflict struct {
	Definition PortDefinition
	// HostPort is the effective (configured or default) port that is busy.
	HostPort int
}

// FindPortConflicts probes the host ports the generated compose file will
// publish and returns those already bound by something other than this
// project's own containers.
func FindPortConflicts(ctx context.Context, projectFolder string, ports shop.ConfigDockerPorts) ([]PortConflict, error) {
	lock, err := composer.ReadLock(filepath.Join(projectFolder, "composer.lock"))
	if err != nil {
		return nil, fmt.Errorf("failed to read composer.lock: %w", err)
	}

	// A hung Docker CLI must not block the startup path indefinitely.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	owned := ownPublishedPorts(ctx, projectFolder)

	return findConflicts(activeDefinitions(lock), ports, owned, func(port int) bool {
		return isPortFree(ctx, port)
	}), nil
}

// activeDefinitions filters PortDefinitions down to the services the compose
// file will actually contain for the given composer.lock.
func activeDefinitions(lock *composer.Lock) []PortDefinition {
	return portDefinitions(ActiveServices(FeaturesFromLock(lock)))
}

// findConflicts reports the ports that are neither disabled, published by our
// own compose project, nor free to bind.
func findConflicts(defs []PortDefinition, ports shop.ConfigDockerPorts, owned map[int]struct{}, isFree func(int) bool) []PortConflict {
	var conflicts []PortConflict
	for _, def := range defs {
		hostPort := HostPort(ports, def.Key)
		if hostPort == 0 {
			continue
		}
		if _, ok := owned[hostPort]; ok {
			continue
		}
		if isFree(hostPort) {
			continue
		}
		conflicts = append(conflicts, PortConflict{Definition: def, HostPort: hostPort})
	}

	return conflicts
}

// isPortFree reports whether a wildcard TCP bind on the port succeeds. Docker
// publishes on 0.0.0.0 and [::] by default, so a dual-stack wildcard bind
// mirrors what `docker compose up` will attempt.
func isPortFree(ctx context.Context, port int) bool {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}

	_ = listener.Close()
	return true
}

// ownPublishedPorts returns the host ports currently published by this
// project's own compose containers, so an already (or partially) running
// environment is not reported as a conflict. Errors are treated as "nothing
// running".
func ownPublishedPorts(ctx context.Context, projectFolder string) map[int]struct{} {
	containers, err := composePS(ctx, projectFolder)
	if err != nil {
		return nil
	}

	owned := make(map[int]struct{})
	for _, container := range containers {
		for _, pub := range container.Publishers {
			if pub.PublishedPort != 0 {
				owned[pub.PublishedPort] = struct{}{}
			}
		}
	}

	return owned
}

// AllocateRandomPorts picks a distinct free host port for every conflict,
// keyed by the docker.ports config key. All listeners stay open until every
// port is chosen so no port is handed out twice.
func AllocateRandomPorts(ctx context.Context, conflicts []PortConflict) (map[string]int, error) {
	ports := make(map[string]int, len(conflicts))
	listeners := make([]net.Listener, 0, len(conflicts))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	for _, conflict := range conflicts {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("allocating a free port for %s: %w", conflict.Definition.Key, err)
		}

		listeners = append(listeners, listener)
		ports[conflict.Definition.Key] = listener.Addr().(*net.TCPAddr).Port
	}

	return ports, nil
}
