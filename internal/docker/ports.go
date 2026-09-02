package docker

import (
	"context"
	"fmt"
	"net"
	"time"
)

// PortConflict reports a host port that is already bound by another process.
type PortConflict struct {
	// Service is the compose service publishing the port.
	Service string
	// Endpoint is the ports key of the busy endpoint.
	Endpoint string
	// Label is the human-readable name of the endpoint.
	Label string
	// HostPort is the effective (configured or default) port that is busy.
	HostPort int
}

// ConfigPath is the docker.services path of the conflicting port, for
// messages that tell the user what to edit.
func (c PortConflict) ConfigPath() string {
	return fmt.Sprintf("docker.services.%s.ports.%s", c.Service, c.Endpoint)
}

// PortConflicts probes the fixed host ports the compose file will publish and
// returns those already bound by something other than this project's own
// containers. Routed services publish nothing in proxy mode and are skipped;
// probe failures count as "free" so `docker compose up` surfaces real errors
// itself.
func (e *Environment) PortConflicts(ctx context.Context) []PortConflict {
	// A hung Docker CLI must not block the startup path indefinitely.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	owned := ownPublishedPorts(ctx, e.root)

	return e.findConflicts(owned, func(port int) bool {
		return isPortFree(ctx, port)
	})
}

// findConflicts reports the fixed published ports of the active, unrouted
// services that are neither disabled, published by our own compose project,
// nor free to bind. Random-port endpoints cannot conflict and are skipped.
func (e *Environment) findConflicts(owned map[int]struct{}, isFree func(int) bool) []PortConflict {
	var conflicts []PortConflict
	for _, svc := range e.activeServices() {
		if len(e.routes(svc)) > 0 {
			continue
		}

		ports := e.ports(svc.Name)
		for _, ep := range svc.Endpoints {
			hostPort := ep.hostPort(ports)
			if hostPort == 0 {
				continue
			}
			if _, ok := owned[hostPort]; ok {
				continue
			}
			if isFree(hostPort) {
				continue
			}
			conflicts = append(conflicts, PortConflict{Service: svc.Name, Endpoint: ep.Name, Label: ep.Label, HostPort: hostPort})
		}
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

// AllocateRandomPorts picks a distinct free host port for every conflict and
// returns one override per conflict, in conflict order. All listeners stay
// open until every port is chosen so no port is handed out twice.
func AllocateRandomPorts(ctx context.Context, conflicts []PortConflict) ([]PortOverride, error) {
	overrides := make([]PortOverride, 0, len(conflicts))
	listeners := make([]net.Listener, 0, len(conflicts))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	for _, conflict := range conflicts {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("allocating a free port for %s: %w", conflict.ConfigPath(), err)
		}

		listeners = append(listeners, listener)
		overrides = append(overrides, PortOverride{
			Service:  conflict.Service,
			Endpoint: conflict.Endpoint,
			HostPort: listener.Addr().(*net.TCPAddr).Port,
		})
	}

	return overrides, nil
}
