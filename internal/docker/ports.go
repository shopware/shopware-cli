package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/shop"
)

// PortDefinition describes one host port the generated compose.yaml publishes.
type PortDefinition struct {
	// Key is the docker.ports config key (shop.DockerPort* constant).
	Key string
	// Service is the compose service publishing the port.
	Service string
	// Label is the human-readable name shown in conflict messages.
	Label string
	// Target is the fixed container-side port.
	Target int
	// Default is the host port used when no override is configured.
	Default int
	// RequiresAMQP/RequiresElasticsearch mark ports of services that are only
	// generated when the matching composer package is installed.
	RequiresAMQP          bool
	RequiresElasticsearch bool
}

// PortDefinitions lists every host port the dev compose file can publish, in
// the order the services appear in the generated file.
var PortDefinitions = []PortDefinition{
	{Key: shop.DockerPortWeb, Service: "web", Label: "Shop (Caddy)", Target: 8000, Default: 8000},
	{Key: shop.DockerPortWebAlt, Service: "web", Label: "Shop (alternative HTTP)", Target: 8080, Default: 8080},
	{Key: shop.DockerPortStorefrontWatcherAssets, Service: "web", Label: "Storefront watcher assets", Target: 9999, Default: 9999},
	{Key: shop.DockerPortStorefrontWatcher, Service: "web", Label: "Storefront watcher", Target: 9998, Default: 9998},
	{Key: shop.DockerPortAdminWatcher, Service: "web", Label: "Admin watcher (Vite)", Target: 5173, Default: 5173},
	{Key: shop.DockerPortAdminWatcherHMR, Service: "web", Label: "Admin watcher HMR", Target: 5773, Default: 5773},
	{Key: shop.DockerPortAdminer, Service: "adminer", Label: "Adminer", Target: 8080, Default: 9080},
	{Key: shop.DockerPortMailerSMTP, Service: "mailer", Label: "Mailpit SMTP", Target: 1025, Default: 1025},
	{Key: shop.DockerPortMailerWeb, Service: "mailer", Label: "Mailpit UI", Target: 8025, Default: 8025},
	{Key: shop.DockerPortAMQPManagement, Service: "lavinmq", Label: "LavinMQ management", Target: 15672, Default: 15672, RequiresAMQP: true},
	{Key: shop.DockerPortAMQP, Service: "lavinmq", Label: "AMQP", Target: 5672, Default: 5672, RequiresAMQP: true},
	{Key: shop.DockerPortElasticsearch, Service: "opensearch", Label: "OpenSearch", Target: 9200, Default: 9200, RequiresElasticsearch: true},
}

// HostPort returns the host port configured for key, falling back to the
// definition default when no override is set. It returns 0 when the port is
// disabled (configured as false) and must not be published.
func HostPort(ports shop.ConfigDockerPorts, key string) int {
	if port, ok := ports[key]; ok {
		if port.Disabled() {
			return 0
		}
		if port > 0 {
			return int(port)
		}
	}

	for _, def := range PortDefinitions {
		if def.Key == key {
			return def.Default
		}
	}

	return 0
}

// PortConflict reports a host port that is already bound by another process.
type PortConflict struct {
	Definition PortDefinition
	// HostPort is the effective (configured or default) port that is busy.
	HostPort int
}

// FindPortConflicts probes the host ports the generated compose file will
// publish and returns those already bound by something other than this
// project's own containers.
func FindPortConflicts(ctx context.Context, projectFolder string, ports shop.ConfigDockerPorts) ([]PortConflict, error) {
	lock, err := composer.ReadLock(filepath.Join(projectFolder, "composer.lock"))
	if err != nil {
		return nil, fmt.Errorf("failed to read composer.lock: %w", err)
	}

	owned := ownPublishedPorts(ctx, projectFolder)

	isFree := func(port int) bool { return isPortFree(ctx, port) }

	return findConflicts(activeDefinitions(lock), ports, owned, isFree), nil
}

// activeDefinitions filters PortDefinitions down to the services the compose
// file will actually contain for the given composer.lock.
func activeDefinitions(lock *composer.Lock) []PortDefinition {
	hasAMQP := lock.GetPackage("symfony/amqp-messenger") != nil
	hasElasticsearch := lock.GetPackage("shopware/elasticsearch") != nil

	defs := make([]PortDefinition, 0, len(PortDefinitions))
	for _, def := range PortDefinitions {
		if def.RequiresAMQP && !hasAMQP {
			continue
		}
		if def.RequiresElasticsearch && !hasElasticsearch {
			continue
		}
		defs = append(defs, def)
	}

	return defs
}

// findConflicts is the pure probing core: a port counts as a conflict when it
// is neither disabled, published by our own compose project, nor free to bind.
func findConflicts(defs []PortDefinition, ports shop.ConfigDockerPorts, owned map[int]struct{}, isFree func(int) bool) []PortConflict {
	var conflicts []PortConflict
	for _, def := range defs {
		hostPort := HostPort(ports, def.Key)
		if hostPort == 0 {
			continue
		}
		if _, ok := owned[hostPort]; ok {
			continue
		}
		if isFree(hostPort) {
			continue
		}
		conflicts = append(conflicts, PortConflict{Definition: def, HostPort: hostPort})
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
// running" — the same output is parsed for the dashboard by discoverCompose in
// internal/tui/dev.
func ownPublishedPorts(ctx context.Context, projectFolder string) map[int]struct{} {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectFolder

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	owned := make(map[int]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		var container struct {
			Publishers []struct {
				PublishedPort int `json:"PublishedPort"`
			} `json:"Publishers"`
		}
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}

		for _, pub := range container.Publishers {
			if pub.PublishedPort != 0 {
				owned[pub.PublishedPort] = struct{}{}
			}
		}
	}

	return owned
}

// AllocateRandomPorts picks a distinct free host port for every conflict. All
// listeners stay open until every port is chosen so no port is handed out
// twice. The result maps the docker.ports config key to the new host port.
func AllocateRandomPorts(ctx context.Context, conflicts []PortConflict) (map[string]int, error) {
	ports := make(map[string]int, len(conflicts))
	listeners := make([]net.Listener, 0, len(conflicts))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	for _, conflict := range conflicts {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("allocating a free port for %s: %w", conflict.Definition.Key, err)
		}

		listeners = append(listeners, listener)
		ports[conflict.Definition.Key] = listener.Addr().(*net.TCPAddr).Port
	}

	return ports, nil
}
