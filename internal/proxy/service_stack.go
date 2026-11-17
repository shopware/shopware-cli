package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

const stackConfigLabel = "com.shopware-cli.proxy-stack-config"

// containerEngine groups capabilities to make the demo's dependencies visible.
// It adds no runtime wrapper. A stack tied to one engine could use that concrete
// engine directly instead of declaring this interface.
type containerEngine interface {
	InspectContainer(context.Context, string) (runtime.ContainerState, error)
	CreateContainer(context.Context, runtime.CreateContainerOptions) (string, error)
	StartContainer(context.Context, string) error
	StopContainer(context.Context, string) error
	RemoveContainer(context.Context, string) error
	EnsureNetwork(context.Context, string) error
	SetNetworkAliases(context.Context, string, string, []string) error
	// Container binds execution to an existing container without starting it.
	Container(string) (runtime.Runtime, error)
}

// ProxyServiceStackOptions selects the shared proxy's configuration and resources.
type ProxyServiceStackOptions struct {
	// Directory is the absolute path to the shared proxy state directory.
	Directory string
	// BaseDomain is the DNS zone served by the proxy. Empty uses DefaultDomain.
	BaseDomain string
	// EngineSocketPath is the host path to a Docker-compatible engine socket.
	// Empty uses /var/run/docker.sock.
	EngineSocketPath string
	// TraefikName is the Traefik container name. Empty uses ContainerName.
	TraefikName string
	// DNSName is the DNS container name. Empty uses DNSContainerName.
	DNSName string
	// NetworkName is the shared network name. Empty uses NetworkName.
	NetworkName string
}

// ProxyServiceStack manages the shared Traefik and DNS containers.
type ProxyServiceStack struct {
	engine  containerEngine
	options ProxyServiceStackOptions
}

var _ runtime.ServiceStack = (*ProxyServiceStack)(nil)

// NewProxyServiceStack validates options without accessing the engine or filesystem.
func NewProxyServiceStack(engine containerEngine, options ProxyServiceStackOptions) (*ProxyServiceStack, error) {
	if engine == nil {
		return nil, errors.New("container engine is required")
	}
	if !filepath.IsAbs(options.Directory) {
		return nil, errors.New("proxy directory must be an absolute path")
	}
	if options.BaseDomain == "" {
		options.BaseDomain = DefaultDomain
	}
	if err := ValidateDomain(options.BaseDomain); err != nil {
		return nil, fmt.Errorf("invalid proxy domain: %w", err)
	}
	if options.EngineSocketPath == "" {
		options.EngineSocketPath = "/var/run/docker.sock"
	}
	if !filepath.IsAbs(options.EngineSocketPath) {
		return nil, errors.New("engine socket path must be absolute")
	}
	if options.TraefikName == "" {
		options.TraefikName = ContainerName
	}
	if options.DNSName == "" {
		options.DNSName = DNSContainerName
	}
	if options.NetworkName == "" {
		options.NetworkName = NetworkName
	}
	for _, name := range []string{options.TraefikName, options.DNSName, options.NetworkName} {
		if strings.HasPrefix(name, "-") || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return nil, fmt.Errorf("invalid resource name %q", name)
		}
	}
	if options.TraefikName == options.DNSName {
		return nil, errors.New("proxy container names must differ")
	}
	return &ProxyServiceStack{engine: engine, options: options}, nil
}

// Up writes the service configuration and reconciles both containers.
func (s *ProxyServiceStack) Up(ctx context.Context) error {
	o := s.options
	if err := writeTraefikDynamicConfig(o.Directory, o.BaseDomain); err != nil {
		return fmt.Errorf("cannot write traefik config: %w", err)
	}
	if err := writeDNSCorefile(o.Directory, o.BaseDomain); err != nil {
		return fmt.Errorf("cannot write dns config: %w", err)
	}
	if err := s.engine.EnsureNetwork(ctx, o.NetworkName); err != nil {
		return fmt.Errorf("cannot ensure proxy network: %w", err)
	}
	if err := s.reconcile(ctx, s.traefikSpec(), configVersionLabel); err != nil {
		return err
	}
	return s.reconcile(ctx, s.dnsSpec(), dnsConfigVersionLabel)
}

func (s *ProxyServiceStack) reconcile(ctx context.Context, spec runtime.CreateContainerOptions, ownerLabel string) error {
	// Hash the effective definition before adding the hash itself.
	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("cannot encode container configuration: %w", err)
	}
	hash := sha256.Sum256(data)
	spec.Labels[stackConfigLabel] = hex.EncodeToString(hash[:])
	state, err := s.engine.InspectContainer(ctx, spec.Name)
	if err != nil && !errors.Is(err, runtime.ErrNotFound) {
		return fmt.Errorf("cannot inspect container %s: %w", spec.Name, err)
	}
	if err == nil {
		if state.Labels[ownerLabel] == "" {
			return fmt.Errorf("cannot replace container %s: container is not owned by proxy", spec.Name)
		}
		current := true
		for key, value := range spec.Labels {
			if state.Labels[key] != value {
				current = false
				break
			}
		}
		if current {
			if state.Running {
				return nil
			}
			if err := s.engine.StartContainer(ctx, spec.Name); err != nil {
				return fmt.Errorf("cannot start container %s: %w", spec.Name, err)
			}
			return nil
		}
		if err := s.engine.RemoveContainer(ctx, spec.Name); err != nil {
			return fmt.Errorf("cannot remove container %s: %w", spec.Name, err)
		}
	}
	if _, err := s.engine.CreateContainer(ctx, spec); err != nil {
		return fmt.Errorf("cannot create container %s: %w", spec.Name, err)
	}
	if err := s.engine.StartContainer(ctx, spec.Name); err != nil {
		return fmt.Errorf("cannot start container %s: %w", spec.Name, err)
	}
	return nil
}

func (s *ProxyServiceStack) traefikSpec() runtime.CreateContainerOptions {
	o := s.options
	return runtime.CreateContainerOptions{
		Name: o.TraefikName, Image: TraefikImage, RestartPolicy: "unless-stopped",
		Labels:   map[string]string{configVersionLabel: configVersion, dnsDomainLabel: o.BaseDomain},
		Networks: []runtime.ContainerNetwork{{Name: o.NetworkName}},
		Mounts: []runtime.ContainerMount{
			{Source: o.EngineSocketPath, Target: "/var/run/docker.sock", ReadOnly: true},
			{Source: filepath.Join(o.Directory, "traefik"), Target: containerConfigDir, ReadOnly: true},
		},
		Ports: []runtime.ContainerPort{
			{HostPort: 80, ContainerPort: 80},
			{HostPort: 443, ContainerPort: 443},
			{HostPort: StorefrontAssetsPort, ContainerPort: StorefrontAssetsPort},
		},
		Args: []string{
			"--ping.manualrouting=true",
			"--providers.docker.exposedbydefault=false",
			"--providers.docker.network=" + o.NetworkName,
			"--providers.file.directory=" + containerConfigDir + "/dynamic",
			"--providers.file.watch=true",
			"--entrypoints.web.address=:80",
			"--entrypoints.web.http.redirections.entrypoint.to=websecure",
			"--entrypoints.web.http.redirections.entrypoint.scheme=https",
			"--entrypoints.websecure.address=:443",
			fmt.Sprintf("--entrypoints.sfassets.address=:%d", StorefrontAssetsPort),
		},
	}
}

func (s *ProxyServiceStack) dnsSpec() runtime.CreateContainerOptions {
	o := s.options
	return runtime.CreateContainerOptions{
		Name: o.DNSName, Image: DNSImage, RestartPolicy: "unless-stopped",
		Labels: map[string]string{dnsConfigVersionLabel: dnsConfigVersion, dnsDomainLabel: o.BaseDomain},
		Mounts: []runtime.ContainerMount{{Source: filepath.Join(o.Directory, "dns", "Corefile"), Target: "/Corefile", ReadOnly: true}},
		Ports: []runtime.ContainerPort{
			{HostIP: "127.0.0.1", HostPort: DNSPort, ContainerPort: 53, Protocol: "udp"},
			{HostIP: "127.0.0.1", HostPort: DNSPort, ContainerPort: 53, Protocol: "tcp"},
		},
		Args: []string{"-conf", "/Corefile"},
	}
}

// Stop stops both containers and retains their resources and shared network.
func (s *ProxyServiceStack) Stop(ctx context.Context) error {
	return s.applyOwned(ctx, "stop", s.engine.StopContainer)
}

// Remove removes both containers while retaining the shared network and host state.
func (s *ProxyServiceStack) Remove(ctx context.Context) error {
	return s.applyOwned(ctx, "remove", s.engine.RemoveContainer)
}

func (s *ProxyServiceStack) applyOwned(ctx context.Context, operation string, action func(context.Context, string) error) error {
	var errs []error
	for _, service := range []struct{ name, label string }{{s.options.TraefikName, configVersionLabel}, {s.options.DNSName, dnsConfigVersionLabel}} {
		state, err := s.engine.InspectContainer(ctx, service.name)
		if errors.Is(err, runtime.ErrNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot inspect container %s: %w", service.name, err))
			continue
		}
		if state.Labels[service.label] == "" {
			errs = append(errs, fmt.Errorf("cannot %s container %s: container is not owned by proxy", operation, service.name))
			continue
		}
		if err := action(ctx, service.name); err != nil && !errors.Is(err, runtime.ErrNotFound) {
			errs = append(errs, fmt.Errorf("cannot %s container %s: %w", operation, service.name, err))
		}
	}
	return errors.Join(errs...)
}

// ReconcileHostnames attaches the proxy hostnames to the shared network.
func (s *ProxyServiceStack) ReconcileHostnames(ctx context.Context, aliases []string) error {
	state, err := s.engine.InspectContainer(ctx, s.options.TraefikName)
	if errors.Is(err, runtime.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect container %s: %w", s.options.TraefikName, err)
	}
	if state.Labels[configVersionLabel] == "" {
		return fmt.Errorf("cannot update container %s: container is not owned by proxy", s.options.TraefikName)
	}
	return s.engine.SetNetworkAliases(ctx, s.options.NetworkName, s.options.TraefikName, aliases)
}

// Service binds execution to a named proxy container without starting it.
func (s *ProxyServiceStack) Service(name string) (runtime.Runtime, error) {
	switch name {
	case "traefik":
		return s.engine.Container(s.options.TraefikName)
	case "dns":
		return s.engine.Container(s.options.DNSName)
	default:
		return nil, fmt.Errorf("unknown proxy service %q", name)
	}
}
