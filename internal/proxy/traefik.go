package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultDomain is the shared domain every project is routed under,
	// e.g. "my-shop.shopware.local".
	DefaultDomain = "shopware.local"
	// ContainerName is the fixed name of the shared Traefik container.
	ContainerName = "shopware-cli-proxy"
	// NetworkName is the shared external Docker network Traefik and every
	// proxied project's services are attached to.
	NetworkName = "shopware-cli-proxy"
	// TraefikImage is the image used for the shared reverse proxy.
	TraefikImage = "traefik:v3"

	// configVersion is stamped on the container as a label; bumping it makes
	// EnsureTraefikRunning recreate containers started by older CLI versions
	// with incompatible flags or mounts.
	configVersion      = "6"
	configVersionLabel = "com.shopware-cli.proxy-config-version"
)

// PingHostname is the hostname of the proxy's own health endpoint under
// baseDomain, served by Traefik itself. `proxy verify` uses it to prove the
// whole chain (DNS, routing, TLS, trust) end to end.
func PingHostname(baseDomain string) string {
	return "proxy." + baseDomain
}

// containerConfigDir is where the state directory's traefik/ folder is
// mounted inside the container. Deliberately NOT /etc/traefik: Traefik
// auto-loads a static config file from there and silently ignores every CLI
// flag when one exists — a stray traefik.yml in the mount must never be able
// to do that.
const containerConfigDir = "/shopware-cli"

// dynamicConfigTemplate is the Traefik file-provider fragment: the server
// certificate plus a router exposing Traefik's ping endpoint on
// https://proxy.<domain>/ping. The file provider watches the directory, so
// Traefik picks up regenerated certificates without a restart.
const dynamicConfigTemplate = `tls:
  certificates:
    - certFile: ` + containerConfigDir + `/certs/cert.pem
      keyFile: ` + containerConfigDir + `/certs/key.pem

http:
  routers:
    proxy-ping:
      rule: Host(` + "`%s`" + `)
      entryPoints: websecure
      tls: {}
      service: ping@internal
`

// writeTraefikDynamicConfig writes the watched dynamic configuration below
// dir (the shared state directory).
func writeTraefikDynamicConfig(dir, baseDomain string) error {
	dynamicDir := filepath.Join(dir, "traefik", "dynamic")
	if err := os.MkdirAll(dynamicDir, 0o700); err != nil {
		return err
	}

	// Traefik is configured exclusively via CLI flags. A static config file
	// at /etc/traefik/traefik.y(a)ml (left behind by earlier versions) would
	// silently take precedence over every flag, so clear any such file from
	// the CLI-owned directory. The same goes for stray dynamic files.
	for _, stale := range []string{
		filepath.Join(dir, "traefik", "traefik.yml"),
		filepath.Join(dir, "traefik", "traefik.yaml"),
		filepath.Join(dynamicDir, "tls.yml"),
	} {
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	config := fmt.Sprintf(dynamicConfigTemplate, PingHostname(baseDomain))

	return os.WriteFile(filepath.Join(dynamicDir, "tls.yaml"), []byte(config), 0o600)
}

// Instance is a project container reachable through the shared proxy.
type Instance struct {
	Container string
	Hostname  string
}

// EnsureNetwork creates the shared Docker network if it does not exist yet.
func EnsureNetwork(ctx context.Context) error {
	if _, err := runDocker(ctx, "network", "inspect", NetworkName); err == nil {
		return nil
	}

	_, err := runDocker(ctx, "network", "create", NetworkName)
	return err
}

// ContainerIsRunning reports whether the shared Traefik container is
// currently running.
func ContainerIsRunning(ctx context.Context) bool {
	out, err := runDocker(ctx, "ps", "--filter", "name=^"+ContainerName+"$", "--filter", "status=running", "--format", "{{.Names}}")
	return err == nil && strings.TrimSpace(out) != ""
}

// containerExists reports whether the shared Traefik container exists,
// running or not.
func containerExists(ctx context.Context) bool {
	out, err := runDocker(ctx, "ps", "-a", "--filter", "name=^"+ContainerName+"$", "--format", "{{.Names}}")
	return err == nil && strings.TrimSpace(out) != ""
}

// EnsureTraefikRunning idempotently starts the shared Traefik container: it
// creates the shared network if needed, writes the TLS configuration,
// restarts an existing up-to-date container, or (re-)creates one bound to
// host ports 80 and 443. It is safe to call from any project at any time.
func EnsureTraefikRunning(ctx context.Context, baseDomain string) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}

	if err := writeTraefikDynamicConfig(dir, baseDomain); err != nil {
		return err
	}

	if containerExists(ctx) && !containerIsCurrent(ctx) {
		if _, err := runDocker(ctx, "rm", "-f", ContainerName); err != nil {
			return err
		}
	}

	if ContainerIsRunning(ctx) {
		return nil
	}

	if err := EnsureNetwork(ctx); err != nil {
		return err
	}

	if containerExists(ctx) {
		_, err := runDocker(ctx, "start", ContainerName)
		return err
	}

	_, err = runDocker(ctx, "run", "-d",
		"--name", ContainerName,
		"--network", NetworkName,
		"--restart", "unless-stopped",
		"--label", configVersionLabel+"="+configVersion,
		"-p", "80:80",
		"-p", "443:443",
		"-v", "/var/run/docker.sock:/var/run/docker.sock:ro",
		"-v", filepath.Join(dir, "traefik")+":"+containerConfigDir+":ro",
		TraefikImage,
		// Enable the ping service without its default route (which would
		// need the "traefik" entrypoint); the dynamic config routes
		// ping@internal via https://proxy.<domain>/ping instead.
		"--ping.manualrouting=true",
		"--providers.docker.exposedbydefault=false",
		"--providers.docker.network="+NetworkName,
		"--providers.file.directory="+containerConfigDir+"/dynamic",
		"--providers.file.watch=true",
		"--entrypoints.web.address=:80",
		"--entrypoints.web.http.redirections.entrypoint.to=websecure",
		"--entrypoints.web.http.redirections.entrypoint.scheme=https",
		"--entrypoints.websecure.address=:443",
	)
	return err
}

// containerIsCurrent reports whether the existing container was created with
// the current configuration version.
func containerIsCurrent(ctx context.Context) bool {
	out, err := runDocker(ctx, "inspect", ContainerName, "--format", "{{index .Config.Labels \""+configVersionLabel+"\"}}")
	return err == nil && strings.TrimSpace(out) == configVersion
}

// RestartTraefik restarts the shared proxy container so it serves a
// regenerated server certificate. Traefik's file provider watches the
// dynamic configuration file, but not the certificate files it references —
// without a restart it would keep the previous certificate in memory.
func RestartTraefik(ctx context.Context) error {
	if !containerExists(ctx) {
		return nil
	}

	_, err := runDocker(ctx, "restart", ContainerName)
	return err
}

// StopTraefik stops and removes the shared Traefik container. It does not
// remove the shared network or touch any project's own containers.
func StopTraefik(ctx context.Context) error {
	if !containerExists(ctx) {
		return nil
	}

	_, err := runDocker(ctx, "rm", "-f", ContainerName)
	return err
}

// RunningInstances lists containers currently attached to the shared proxy
// network, for `project proxy list`/`status`.
func RunningInstances(ctx context.Context) ([]Instance, error) {
	out, err := runDocker(ctx, "ps", "--filter", "network="+NetworkName, "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}

	var instances []Instance
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == "" || name == ContainerName {
			continue
		}

		instances = append(instances, Instance{Container: name})
	}

	return instances, nil
}
