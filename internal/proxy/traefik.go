package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings holds the persisted configuration of the shared proxy.
type Settings struct {
	// Domain is the base domain, instances get <name>.<domain> hostnames.
	Domain string `json:"domain"`
	// Hosts contains explicitly registered hostnames outside <name>.<domain>.
	Hosts []string `json:"hosts,omitempty"`
	// HTTPPort is the host port the proxy listens on for HTTP.
	HTTPPort int `json:"http_port"`
	// HTTPSPort is the host port the proxy listens on for HTTPS.
	HTTPSPort int `json:"https_port"`
}

// DefaultSettings returns the settings used when the proxy has never been configured.
func DefaultSettings() Settings {
	return Settings{
		Domain:    DefaultDomain,
		HTTPPort:  80,
		HTTPSPort: 443,
	}
}

const traefikStaticConfigTemplate = `# Managed by shopware-cli project proxy. Manual changes will be overwritten.
providers:
  docker:
    exposedByDefault: false
    network: %s
  file:
    directory: /etc/traefik/dynamic
    watch: true

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: "%s"
          scheme: https
  websecure:
    address: ":443"

api:
  dashboard: false

log:
  level: INFO
`

const traefikDynamicTLSConfig = `# Managed by shopware-cli project proxy. Manual changes will be overwritten.
tls:
  certificates:
    - certFile: /etc/traefik/certs/cert.pem
      keyFile: /etc/traefik/certs/key.pem
  stores:
    default:
      defaultCertificate:
        certFile: /etc/traefik/certs/cert.pem
        keyFile: /etc/traefik/certs/key.pem
`

// TraefikStaticConfig renders the static Traefik configuration. The HTTP to
// HTTPS redirect has to point at the port the user reaches the proxy on.
func TraefikStaticConfig(settings Settings) string {
	redirectTarget := "websecure"
	if settings.HTTPSPort != 443 {
		redirectTarget = fmt.Sprintf(":%d", settings.HTTPSPort)
	}

	return fmt.Sprintf(traefikStaticConfigTemplate, NetworkName, redirectTarget)
}

// WriteTraefikConfig writes the static and dynamic Traefik configuration into the proxy directory.
func WriteTraefikConfig(dir string, settings Settings) error {
	dynamicDir := filepath.Join(dir, "traefik", "dynamic")
	if err := os.MkdirAll(dynamicDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "traefik", "traefik.yml"), []byte(TraefikStaticConfig(settings)), 0o644); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dynamicDir, "tls.yml"), []byte(traefikDynamicTLSConfig), 0o644)
}

// EnsureNetwork creates the shared Docker network when it does not exist yet.
func EnsureNetwork(ctx context.Context) error {
	if _, err := runDocker(ctx, "network", "inspect", NetworkName); err == nil {
		return nil
	}

	if _, err := runDocker(ctx, "network", "create", NetworkName); err != nil {
		return fmt.Errorf("create docker network %s: %w", NetworkName, err)
	}

	return nil
}

// StartContainer (re-)creates and starts the shared Traefik container.
func StartContainer(ctx context.Context, dir string, settings Settings) error {
	// Remove a previous instance so configuration changes always apply.
	_, _ = runDocker(ctx, "rm", "--force", ContainerName)

	args := []string{
		"run",
		"--detach",
		"--name", ContainerName,
		"--restart", "unless-stopped",
		"--network", NetworkName,
		"--publish", fmt.Sprintf("%d:80", settings.HTTPPort),
		"--publish", fmt.Sprintf("%d:443", settings.HTTPSPort),
		"--volume", "/var/run/docker.sock:/var/run/docker.sock:ro",
		"--volume", filepath.Join(dir, "traefik", "traefik.yml") + ":/etc/traefik/traefik.yml:ro",
		"--volume", filepath.Join(dir, "traefik", "dynamic") + ":/etc/traefik/dynamic:ro",
		"--volume", filepath.Join(dir, "traefik", "certs") + ":/etc/traefik/certs:ro",
		TraefikImage,
	}

	if _, err := runDocker(ctx, args...); err != nil {
		return fmt.Errorf("start proxy container: %w", err)
	}

	return nil
}

// StopContainer removes the shared Traefik container. It is not an error when
// the container does not exist.
func StopContainer(ctx context.Context) error {
	if !ContainerExists(ctx) {
		return nil
	}

	if _, err := runDocker(ctx, "rm", "--force", ContainerName); err != nil {
		return fmt.Errorf("remove proxy container: %w", err)
	}

	return nil
}

// RestartContainer restarts the proxy container when it is running, so that
// renewed certificates are picked up.
func RestartContainer(ctx context.Context) error {
	if !ContainerIsRunning(ctx) {
		return nil
	}

	if _, err := runDocker(ctx, "restart", ContainerName); err != nil {
		return fmt.Errorf("restart proxy container: %w", err)
	}

	return nil
}

// ContainerExists reports whether the proxy container exists (running or not).
func ContainerExists(ctx context.Context) bool {
	_, err := runDocker(ctx, "container", "inspect", "--format", "{{.State.Status}}", ContainerName)

	return err == nil
}

// ContainerIsRunning reports whether the proxy container is currently running.
func ContainerIsRunning(ctx context.Context) bool {
	out, err := runDocker(ctx, "container", "inspect", "--format", "{{.State.Status}}", ContainerName)

	return err == nil && strings.TrimSpace(out) == "running"
}

// Instance describes a currently running proxied project container.
type Instance struct {
	Container string
	Host      string
}

// RunningInstances lists all running containers that carry the proxy host label.
func RunningInstances(ctx context.Context) ([]Instance, error) {
	format := fmt.Sprintf(`{{.Names}}\t{{.Label %q}}`, HostLabel)

	out, err := runDocker(ctx, "ps", "--filter", "label="+HostLabel, "--format", format)
	if err != nil {
		return nil, err
	}

	var instances []Instance

	for line := range strings.Lines(out) {
		name, host, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found || host == "" {
			continue
		}

		instances = append(instances, Instance{Container: name, Host: host})
	}

	return instances, nil
}
