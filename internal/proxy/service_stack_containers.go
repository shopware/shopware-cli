package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var _ runtime.ServiceStackContainers = (*ProxyServiceStack)(nil)

// Containers resolves the proxy's existing owned containers without starting them.
func (s *ProxyServiceStack) Containers(ctx context.Context) ([]runtime.ServiceContainer, error) {
	var containers []runtime.ServiceContainer
	for _, service := range []struct{ service, name, label string }{
		{"traefik", s.options.TraefikName, configVersionLabel},
		{"dns", s.options.DNSName, dnsConfigVersionLabel},
	} {
		state, err := s.engine.InspectContainer(ctx, service.name)
		if errors.Is(err, runtime.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot inspect container %s: %w", service.name, err)
		}
		if state.Labels[service.label] == "" {
			return nil, fmt.Errorf("cannot identify container %s: container is not owned by proxy", service.name)
		}
		if state.ID == "" {
			return nil, fmt.Errorf("cannot identify container %s: empty container ID", service.name)
		}
		containers = append(containers, runtime.ServiceContainer{Service: service.service, ID: state.ID})
	}
	return containers, nil
}
