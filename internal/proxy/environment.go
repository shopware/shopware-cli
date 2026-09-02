package proxy

import (
	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

// NewEnvironment resolves the project's Docker dev environment for its
// effective run mode: proxied when the config names a hostname under the proxy
// base domain, plain fixed-port otherwise. proxyFallback forces plain mode for
// a project whose config still names the proxy hostname (a failed proxy
// bootstrap, or `proxy down` reverting it). It is the single mode-aware entry
// point every generic caller (project dev, the dev TUI) uses, so regenerating
// the compose file can never silently drop a project out of proxy mode.
func NewEnvironment(projectRoot string, cfg *shop.Config, proxyFallback bool) (*docker.Environment, error) {
	opts := cfg.DockerOptions()

	if !proxyFallback {
		baseDomain := BaseDomain()
		if IsProxyProjectForDomain(cfg, baseDomain) {
			// A hostname that cannot be derived leaves the project in plain
			// mode, as before.
			if hostname, err := ProjectHostname(projectRoot, cfg, baseDomain); err == nil {
				opts.Proxy = proxyFor(projectRoot, hostname, opts)
			}
		}
	}

	return docker.NewEnvironment(projectRoot, opts)
}

// NewProxiedEnvironment builds the project's environment in proxy mode at an
// explicit hostname. `proxy up` needs it because it regenerates the compose
// file before the config records the hostname.
func NewProxiedEnvironment(projectRoot string, cfg *shop.Config, hostname string) (*docker.Environment, error) {
	opts := cfg.DockerOptions()
	opts.Proxy = proxyFor(projectRoot, hostname, opts)

	return docker.NewEnvironment(projectRoot, opts)
}

// proxyFor derives the proxy settings for a project deterministically from
// the hostname and machine settings: shared network, combined CA bundle path
// and the admin watcher's dev-server port.
func proxyFor(projectRoot, hostname string, opts docker.Options) *docker.Proxy {
	// Reference the deterministic bundle path; the file itself is written by
	// PrepareInfra before the containers start. Best-effort: if the state dir is
	// unavailable the mount is simply dropped (the shop still serves, only its
	// own TLS self-calls would be untrusted).
	bundlePath, _ := ContainerCABundlePath(opts.PHP.WebImage())

	return &docker.Proxy{
		Hostname:       hostname,
		NetworkName:    NetworkName,
		CABundlePath:   bundlePath,
		AdminWatchPort: extension.AdminDevServerPort(projectRoot),
	}
}
