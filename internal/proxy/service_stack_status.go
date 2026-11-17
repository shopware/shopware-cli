package proxy

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Status reports whether the proxy's required containers are running.
func (s *ProxyServiceStack) Status(ctx context.Context) (runtime.ServiceStackStatus, error) {
	running := 0
	for _, service := range []struct{ name, label string }{{s.options.TraefikName, configVersionLabel}, {s.options.DNSName, dnsConfigVersionLabel}} {
		state, err := s.engine.InspectContainer(ctx, service.name)
		if errors.Is(err, runtime.ErrNotFound) {
			continue
		}
		if err != nil {
			return runtime.ServiceStackUnknown, fmt.Errorf("cannot inspect container %s: %w", service.name, err)
		}
		if state.Labels[service.label] == "" {
			return runtime.ServiceStackUnknown, fmt.Errorf("cannot inspect container %s: container is not owned by proxy", service.name)
		}
		if state.Status == runtime.ContainerUnknown {
			return runtime.ServiceStackUnknown, fmt.Errorf("cannot inspect container %s: unknown container status", service.name)
		}
		if state.Status == runtime.ContainerRunning {
			running++
		}
	}
	switch running {
	case 0:
		return runtime.ServiceStackStopped, nil
	case 2:
		return runtime.ServiceStackRunning, nil
	default:
		return runtime.ServiceStackPartial, nil
	}
}
