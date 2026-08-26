package proxy

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

// InfraParams carries the project-specific inputs PrepareInfra needs. The
// caller resolves them from the project config and machine settings.
type InfraParams struct {
	CanonicalRoot string
	Hostname      string
	BaseDomain    string
	// ConfigPath names the project config file; used only in the hostname
	// collision hint.
	ConfigPath string
	// Image is the docker-dev image the shop's containers run. PrepareInfra
	// reads its system CA bundle to build the combined trust bundle.
	Image string
}

// PrepareInfra brings up the shared infrastructure a proxy project needs before
// its containers start: it checks the hostname is free, ensures the server
// certificate, the shared Traefik container and the DNS container. It does not
// write the project's compose file (WriteComposeFile does, in proxy mode) nor
// register the project — the caller layers that on top. It returns the
// certificate info so callers can react to a freshly created CA. Safe to call
// repeatedly.
func PrepareInfra(ctx context.Context, p InfraParams, reg Registry) (CertInfo, error) {
	if other, found := reg.FindByHostname(p.Hostname, p.CanonicalRoot); found {
		return CertInfo{}, fmt.Errorf("hostname %s is already registered to %s, set a different \"url\" in %s to disambiguate", p.Hostname, other.ProjectRoot, p.ConfigPath)
	}

	certInfo, err := ensureCertificate(p.Hostname, p.BaseDomain, reg)
	if err != nil {
		return CertInfo{}, err
	}

	// Build the combined CA bundle the shop's containers mount over their system
	// trust store, so PHP/curl/Node trust the proxy's HTTPS certificates. Done
	// before the containers start, so the mount target always exists.
	if _, err := EnsureContainerCABundle(ctx, p.Image, certInfo.CAPath); err != nil {
		return CertInfo{}, fmt.Errorf("building container CA bundle: %w", err)
	}

	if err := EnsureTraefikRunning(ctx, p.BaseDomain); err != nil {
		return CertInfo{}, err
	}
	// A regenerated certificate (e.g. new project wildcard SANs) is only served
	// after a restart.
	if certInfo.Changed {
		if err := RestartTraefik(ctx); err != nil {
			return CertInfo{}, err
		}
	}

	if err := EnsureDNSContainerRunning(ctx, p.BaseDomain); err != nil {
		return CertInfo{}, fmt.Errorf("starting DNS server: %w", err)
	}

	return certInfo, nil
}

// ensureCertificate makes sure the shared server certificate covers this
// project's hostname (and its wildcard) plus every already-registered
// project's, so all shops are served by one trusted certificate.
func ensureCertificate(hostname, baseDomain string, reg Registry) (CertInfo, error) {
	extraHosts := []string{hostname, "*." + hostname}
	for _, p := range reg.Projects {
		extraHosts = append(extraHosts, p.Hostname, "*."+p.Hostname)
	}

	dir, err := StateDir()
	if err != nil {
		return CertInfo{}, err
	}

	return EnsureCertificate(dir, CertHosts(baseDomain, extraHosts))
}

// WriteComposeFile writes the project's compose.yaml, generating it in shared-
// proxy mode when the project's config points at a hostname under the proxy
// base domain, and in plain fixed-port mode otherwise. It is the single proxy-
// aware entry point every generic caller (project dev, the dev TUI) uses, so
// regenerating the compose file can never silently drop a project out of proxy
// mode. Proxy up/down, which toggle the mode explicitly, build the options
// themselves instead.
func WriteComposeFile(projectRoot string, cfg *shop.Config) error {
	opts := docker.ComposeOptionsFromConfig(cfg)

	if p, ok := ComposeProxyOptions(projectRoot, cfg); ok {
		if opts == nil {
			opts = &docker.ComposeOptions{}
		}
		opts.Proxy = p
	}

	return docker.WriteComposeFile(projectRoot, opts)
}

// ComposeProxyOptions returns the docker proxy options for a project when it is
// a proxy project (its configured URL is a hostname under the proxy base
// domain), deriving everything deterministically from the config and machine
// settings — hostname, shared network, root CA path, APP_URL and the admin
// watcher's dev-server port. The second result is false for port-based
// projects.
func ComposeProxyOptions(projectRoot string, cfg *shop.Config) (*docker.ProxyOptions, bool) {
	baseDomain := BaseDomain()
	if !IsProxyProjectForDomain(cfg, baseDomain) {
		return nil, false
	}

	hostname, err := ProjectHostname(projectRoot, cfg, baseDomain)
	if err != nil {
		return nil, false
	}

	// Reference the deterministic bundle path; the file itself is written by
	// PrepareInfra before the containers start. Best-effort: if the state dir is
	// unavailable the mount is simply dropped (the shop still serves, only its
	// own TLS self-calls would be untrusted).
	bundlePath, _ := ContainerCABundlePath(docker.WebImage(docker.ComposeOptionsFromConfig(cfg)))

	return &docker.ProxyOptions{
		Hostname:       hostname,
		NetworkName:    NetworkName,
		CABundlePath:   bundlePath,
		AdminWatchPort: extension.AdminDevServerPort(projectRoot),
	}, true
}

// IsProxyProject reports whether the project is configured to be served at a
// stable hostname under the shared proxy's base domain (the signal
// `project create --local-domain` and `proxy up` write into
// .shopware-project.yml), as opposed to a fixed localhost port.
func IsProxyProject(cfg *shop.Config) bool {
	return IsProxyProjectForDomain(cfg, BaseDomain())
}

// IsProxyProjectForDomain is the pure core of IsProxyProject with the base
// domain passed in, so it can be tested without depending on stored settings.
func IsProxyProjectForDomain(cfg *shop.Config, baseDomain string) bool {
	if cfg == nil {
		return false
	}

	// Resolve the url the same way the executor does.
	effective := cfg.EffectiveURL()

	if effective == "" {
		return false
	}

	parsed, err := url.Parse(effective)
	if err != nil {
		return false
	}

	host := parsed.Hostname()
	return host == baseDomain || strings.HasSuffix(host, "."+baseDomain)
}
