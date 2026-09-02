package proxy

import (
	"context"

	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/shop"
)

// ApplyRandomPorts remaps every conflicting host port to a random free one:
// it regenerates compose.yaml with the overrides and persists them to the
// local config override so future runs reuse them. The overrides come back in
// conflict order. The returned config carries them; cfg itself is left
// untouched so callers can adopt the result only once every write succeeded.
func ApplyRandomPorts(ctx context.Context, projectRoot, configPath string, cfg *shop.Config, proxyFallback bool, conflicts []docker.PortConflict) (*shop.Config, []docker.PortOverride, error) {
	overrides, err := docker.AllocateRandomPorts(ctx, conflicts)
	if err != nil {
		return nil, nil, err
	}

	updated := cfg.WithDockerPortOverrides(overrides)

	env, err := NewEnvironment(projectRoot, updated, proxyFallback)
	if err != nil {
		return nil, nil, err
	}
	if err := env.WriteCompose(); err != nil {
		return nil, nil, err
	}

	if err := shop.UpdateLocalDockerPorts(configPath, overrides); err != nil {
		return nil, nil, err
	}

	return updated, overrides, nil
}
