package docker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
)

// DiscoveredService is an auxiliary service of the running development
// environment with the URL and credentials it is reached with from the host.
type DiscoveredService struct {
	Name     string
	URL      string
	Username string
	Password string
}

// BackgroundProcess is a long-running compose service without a published port
// (the messenger worker and scheduled-task runner). Running reflects whether
// its container is currently up.
type BackgroundProcess struct {
	Name    string
	Running bool
}

// Environment is the running development environment's containers, classified
// by role.
type Environment struct {
	// Services are the auxiliary services reachable from the host.
	Services []DiscoveredService
	// Background are the long-running console processes.
	Background []BackgroundProcess
	// WebPort is the host port the shop is published on, 0 when it cannot be
	// determined (environment down, web container not publishing).
	WebPort int
}

// overrideService describes a service the generator never emits but users add
// via compose.override.yaml, so it still shows up in the service list.
type overrideService struct {
	Name       string
	TargetPort int
	Username   string
	Password   string
}

var overrideServices = map[string]overrideService{
	"rabbitmq": {Name: "Queue (RabbitMQ)", TargetPort: 15672, Username: "guest", Password: "guest"},
}

// backgroundServiceLabel returns the display label for a compose service that
// is one of the dedicated background processes, and whether it is one.
func backgroundServiceLabel(service string) (string, bool) {
	for _, bg := range BackgroundServices {
		if bg.Name == service {
			return bg.Label, true
		}
	}
	return "", false
}

// composeCommand builds a `docker compose <args...>` invocation rooted at
// projectRoot. Unless the environment already pins COMPOSE_PROJECT_NAME, the
// project name is pinned explicitly: compose re-reads the project .env per
// invocation, and pinning guarantees we only ever see this project's own
// containers.
func composeCommand(ctx context.Context, projectRoot string, args ...string) *exec.Cmd {
	fullArgs := []string{"compose"}
	if os.Getenv("COMPOSE_PROJECT_NAME") == "" {
		if name := shop.ReadComposeProjectName(projectRoot); name != "" {
			fullArgs = append(fullArgs, "-p", name)
		}
	}

	cmd := exec.CommandContext(ctx, "docker", append(fullArgs, args...)...)
	cmd.Dir = projectRoot
	return cmd
}

// composePSPublisher is one published port of a composePSContainer.
type composePSPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// composePSContainer is one container entry of `docker compose ps --format
// json` (NDJSON, one object per line).
type composePSContainer struct {
	Name       string               `json:"Name"`
	Service    string               `json:"Service"`
	State      string               `json:"State"`
	Publishers []composePSPublisher `json:"Publishers"`
}

// publishedPorts flattens the container's publishers into a
// container-port → host-port map.
func (c composePSContainer) publishedPorts() map[int]int {
	ports := make(map[int]int, len(c.Publishers))
	for _, pub := range c.Publishers {
		if pub.PublishedPort != 0 {
			ports[pub.TargetPort] = pub.PublishedPort
		}
	}
	return ports
}

// composePS lists the project's containers via `docker compose ps`.
func composePS(ctx context.Context, projectRoot string) ([]composePSContainer, error) {
	output, err := composeCommand(ctx, projectRoot, "ps", "--format", "json").Output()
	if err != nil {
		return nil, err
	}

	var containers []composePSContainer
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		var container composePSContainer
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}
		containers = append(containers, container)
	}

	return containers, nil
}

// DiscoverEnvironment inspects the project's containers via `docker compose
// ps` and classifies them into published auxiliary services, background
// processes, and the shop's published web port. proxyHost is the project's
// proxy hostname ("" in plain-port mode): proxied services publish no host
// port and are reached at their subdomain instead.
func DiscoverEnvironment(ctx context.Context, projectRoot, proxyHost string) (Environment, error) {
	containers, err := composePS(ctx, projectRoot)
	if err != nil {
		return Environment{}, err
	}

	return classifyContainers(containers, proxyHost), nil
}

// classifyContainers sorts raw compose containers into the environment's
// services, background processes, and published web port.
func classifyContainers(containers []composePSContainer, proxyHost string) Environment {
	var env Environment

	for _, container := range containers {
		if label, ok := backgroundServiceLabel(container.Service); ok {
			env.Background = append(env.Background, BackgroundProcess{
				Name:    label,
				Running: container.State == "running",
			})
			continue
		}

		ports := container.publishedPorts()

		if container.Service == "web" {
			if port, ok := ports[ShopEndpoint().ContainerPort]; ok {
				env.WebPort = port
			}
		}

		if def := ServiceByName(container.Service); def != nil {
			if def.Hidden {
				continue
			}

			// Prefer a real published port when the service has one (plain
			// mode). Proxied services publish nothing and resolve to their
			// proxy subdomain instead.
			url := def.AccessURL(ports, proxyHost)
			if url == "" {
				continue
			}

			env.Services = append(env.Services, DiscoveredService{
				Name:     def.Label,
				URL:      url,
				Username: def.Username,
				Password: def.Password,
			})
			continue
		}

		// Services the generator never emits (user compose overrides) only
		// have a URL when they publish their target port.
		extra, ok := overrideServices[container.Service]
		if !ok {
			continue
		}
		publishedPort, hasPort := ports[extra.TargetPort]
		if !hasPort {
			continue
		}
		env.Services = append(env.Services, DiscoveredService{
			Name:     extra.Name,
			URL:      loopbackHTTPURL(publishedPort),
			Username: extra.Username,
			Password: extra.Password,
		})
	}

	return env
}
