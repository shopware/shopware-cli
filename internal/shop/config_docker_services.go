package shop

import (
	"maps"

	"github.com/invopop/jsonschema"

	"github.com/shopware/shopware-cli/internal/docker"
)

// ConfigDockerServices holds the per-service settings of the Docker dev
// environment, keyed by compose service name. Valid names and keys are
// defined by the service catalog in internal/docker.
type ConfigDockerServices map[string]*ConfigDockerService

// ConfigDockerService configures one compose service of the dev environment;
// it is the catalog's settings type so the generator reads it directly.
type ConfigDockerService = docker.ServiceSettings

// Ports returns the host-port overrides of a service, nil when none are
// configured. Nil-safe.
func (s ConfigDockerServices) Ports(name string) docker.Ports {
	if svc := s[name]; svc != nil {
		return svc.Ports
	}
	return nil
}

// validate rejects services, endpoints and types the catalog does not know,
// so a typo fails at read time instead of being silently ignored.
func (s ConfigDockerServices) validate() error {
	return docker.ValidateSettings(docker.Settings(s))
}

// JSONSchemaExtend replaces the generic map schema with one property per
// configurable catalog service, each allowing only the keys that apply to it.
func (ConfigDockerServices) JSONSchemaExtend(s *jsonschema.Schema) {
	schema := docker.SettingsSchema()
	s.Type = schema.Type
	s.Properties = schema.Properties
	s.AdditionalProperties = schema.AdditionalProperties
	s.PatternProperties = nil
}

// DockerOptions maps the configuration onto the dev environment's options:
// PHP settings and per-service settings. The lock features, user and worker
// are resolved by docker.NewEnvironment, the proxy mode by
// proxy.NewEnvironment. Nil-safe for a nil config.
func (c *Config) DockerOptions() docker.Options {
	var opts docker.Options
	if c == nil || c.Docker == nil {
		return opts
	}

	if php := c.Docker.PHP; php != nil {
		opts.PHP = docker.PHP{
			Version:              php.Version,
			Profiler:             php.Profiler,
			BlackfireServerID:    php.BlackfireServerID,
			BlackfireServerToken: php.BlackfireServerToken,
			TidewaysAPIKey:       php.TidewaysAPIKey,
		}
	}
	opts.Services = docker.Settings(c.Docker.Services)

	return opts
}

// DockerServices returns the configured per-service settings, nil when none
// are set. Nil-safe for a nil config.
func (c *Config) DockerServices() ConfigDockerServices {
	if c == nil || c.Docker == nil {
		return nil
	}
	return c.Docker.Services
}

// DockerPorts returns the host-port overrides of a service, nil when none are
// configured. Nil-safe for a nil config.
func (c *Config) DockerPorts(service string) docker.Ports {
	return c.DockerServices().Ports(service)
}

// WithDockerPortOverrides returns a copy of the config with the host-port
// overrides merged into docker.services. The receiver and its docker section
// are left untouched, so a caller can build the new config off the update
// thread and adopt it once every dependent write succeeded. An empty override
// set returns the receiver itself.
func (c *Config) WithDockerPortOverrides(overrides []docker.PortOverride) *Config {
	if len(overrides) == 0 {
		return c
	}

	services := c.DockerServices().clone()
	if services == nil {
		services = ConfigDockerServices{}
	}
	for _, o := range overrides {
		svc := services[o.Service]
		if svc == nil {
			svc = &ConfigDockerService{}
			services[o.Service] = svc
		}
		if svc.Ports == nil {
			svc.Ports = docker.Ports{}
		}
		svc.Ports[o.Endpoint] = docker.Port(o.HostPort)
	}

	return c.withDockerServices(services)
}

// withoutDockerPorts returns the config with every service's port overrides
// removed, copying the docker section only when something is removed.
func (c *Config) withoutDockerPorts() *Config {
	services := c.DockerServices()
	hasPorts := false
	for _, svc := range services {
		if svc != nil && svc.Ports != nil {
			hasPorts = true
			break
		}
	}
	if !hasPorts {
		return c
	}

	stripped := services.clone()
	for name, svc := range stripped {
		// An entry without settings (e.g. "adminer:" with no value) decodes to
		// nil; it has no ports to strip and stays as written.
		if svc == nil {
			continue
		}
		svc.Ports = nil
		if svc.Type == "" && svc.Version == "" {
			delete(stripped, name)
		}
	}
	if len(stripped) == 0 {
		stripped = nil
	}

	return c.withDockerServices(stripped)
}

// clone deep-copies the services map, so edits never leak into the receiver.
func (s ConfigDockerServices) clone() ConfigDockerServices {
	if s == nil {
		return nil
	}

	out := make(ConfigDockerServices, len(s))
	for name, svc := range s {
		if svc == nil {
			out[name] = nil
			continue
		}
		copied := *svc
		copied.Ports = maps.Clone(svc.Ports)
		out[name] = &copied
	}

	return out
}

// withDockerServices returns a shallow copy of the config whose docker section
// is copied as well and carries the given services.
func (c *Config) withDockerServices(services ConfigDockerServices) *Config {
	cfg := *c
	docker := ConfigDocker{}
	if c.Docker != nil {
		docker = *c.Docker
	}
	docker.Services = services
	cfg.Docker = &docker

	return &cfg
}
