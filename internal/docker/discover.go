package docker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/shopware/shopware-cli/internal/envfile"
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

// RunningEnvironment is the running development environment's containers, classified
// by role.
type RunningEnvironment struct {
	// Services are the auxiliary services reachable from the host.
	Services []DiscoveredService
	// Background are the long-running console processes.
	Background []BackgroundProcess
	// WebPort is the host port the shop is published on, 0 when it cannot be
	// determined (environment down, web container not publishing).
	WebPort int
}

// composeCommand builds a `docker compose <args...>` invocation rooted at
// projectRoot. Unless the environment already pins COMPOSE_PROJECT_NAME, the
// project name is pinned explicitly: compose re-reads the project .env per
// invocation, and pinning guarantees we only ever see this project's own
// containers.
func composeCommand(ctx context.Context, projectRoot string, args ...string) *exec.Cmd {
	fullArgs := []string{"compose"}
	if os.Getenv("COMPOSE_PROJECT_NAME") == "" {
		if name := envfile.ReadComposeProjectName(projectRoot); name != "" {
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
	// --all includes stopped containers, so background processes report their
	// real state instead of silently disappearing from the list.
	output, err := composeCommand(ctx, projectRoot, "ps", "--all", "--format", "json").Output()
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

// Discover inspects the project's containers via `docker compose ps` and
// classifies them into published auxiliary services, background processes,
// and the shop's published web port. Proxied services publish no host port and
// are reached at their subdomain instead.
func (e *Environment) Discover(ctx context.Context) (RunningEnvironment, error) {
	containers, err := composePS(ctx, e.root)
	if err != nil {
		return RunningEnvironment{}, err
	}

	return classifyContainers(containers, e.proxyHost()), nil
}

// classifyContainers sorts raw compose containers into the environment's
// services, background processes, and published web port.
func classifyContainers(containers []composePSContainer, proxyHost string) RunningEnvironment {
	var env RunningEnvironment
	shopPort := mustEndpoint(ServiceWeb, PortHTTP).ContainerPort

	for _, container := range containers {
		def := byName(container.Service)

		if def != nil && def.Background {
			env.Background = append(env.Background, BackgroundProcess{
				Name:    def.Label,
				Running: container.State == "running",
			})
			continue
		}

		ports := container.publishedPorts()

		if container.Service == "web" {
			if port, ok := ports[shopPort]; ok {
				env.WebPort = port
			}
		}

		// Only running services are reachable; a stopped proxied service would
		// otherwise still resolve to its subdomain.
		if def == nil || def.Hidden || container.State != "running" {
			continue
		}

		// Prefer a real published port when the service has one (plain mode).
		// Proxied services publish nothing and resolve to their proxy subdomain
		// instead; services without either are unreachable and skipped.
		url := def.publishedURL(ports, proxyHost)
		if url == "" {
			continue
		}

		env.Services = append(env.Services, DiscoveredService{
			Name:     def.Label,
			URL:      url,
			Username: def.Username,
			Password: def.Password,
		})
	}

	return env
}
