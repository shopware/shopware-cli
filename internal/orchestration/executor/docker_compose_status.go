package executor

import (
	"context"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Status reports whether the configured services are running, without checking readiness.
func (p *DockerCompose) Status(ctx context.Context) (runtime.ServiceStackStatus, error) {
	configured, err := p.serviceNames(ctx, "config", "--services")
	if err != nil {
		return runtime.ServiceStackUnknown, err
	}
	services := make(map[string]bool, len(configured))
	for _, name := range configured {
		services[name] = false
	}
	running, err := p.serviceNames(ctx, "ps", "--services", "--status", "running", "--orphans=false")
	if err != nil {
		return runtime.ServiceStackUnknown, err
	}
	for _, name := range running {
		if _, required := services[name]; required {
			services[name] = true
		}
	}
	count := 0
	for _, running := range services {
		if running {
			count++
		}
	}
	switch {
	case count == 0:
		return runtime.ServiceStackStopped, nil
	case count == len(services):
		return runtime.ServiceStackRunning, nil
	default:
		return runtime.ServiceStackPartial, nil
	}
}

func (p *DockerCompose) serviceNames(ctx context.Context, args ...string) ([]string, error) {
	output, err := p.query(ctx, args...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}
