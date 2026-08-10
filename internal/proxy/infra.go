package proxy

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/docker"
)

// InfraParams carries the project-specific inputs PrepareInfra needs. The
// caller resolves them and computes AdminWatchPort (via
// extension.AdminDevServerPort), so this package needs no Shopware-version or
// admin-watch logic of its own.
type InfraParams struct {
	ProjectRoot   string
	CanonicalRoot string
	Hostname      string
	BaseDomain    string
	// ConfigPath names the project config file; used only in the hostname
	// collision hint.
	ConfigPath string
	// AdminWatchPort is the container port the admin watcher's dev server binds.
	AdminWatchPort int
}

// PrepareInfra brings up everything the shared proxy needs before a project's
// containers start: it checks the hostname is free, verifies docker compose
// supports resets, ensures the server certificate, the shared Traefik container
// and the DNS container, and writes the compose override with APP_URL pinned
// (so PHP renders absolute URLs — e.g. the storefront import map — with the
// proxy hostname, not the stale image default). It returns the certificate info
// so callers can react to a freshly created CA. It neither starts nor registers
// the project: the caller layers start/registration on top. Safe to call
// repeatedly.
func PrepareInfra(ctx context.Context, p InfraParams, reg Registry) (CertInfo, error) {
	if other, found := reg.FindByHostname(p.Hostname, p.CanonicalRoot); found {
		return CertInfo{}, fmt.Errorf("hostname %s is already registered to %s, set a different \"url\" in %s to disambiguate", p.Hostname, other.ProjectRoot, p.ConfigPath)
	}

	if err := docker.EnsureComposeSupportsReset(ctx); err != nil {
		return CertInfo{}, err
	}

	certInfo, err := ensureCertificate(p.Hostname, p.BaseDomain, reg)
	if err != nil {
		return CertInfo{}, err
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

	if err := docker.WriteComposeOverride(p.ProjectRoot, &docker.ProxyOptions{
		Hostname:       p.Hostname,
		NetworkName:    NetworkName,
		CAPath:         certInfo.CAPath,
		AppURL:         "https://" + p.Hostname,
		AdminWatchPort: p.AdminWatchPort,
	}); err != nil {
		return CertInfo{}, err
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
