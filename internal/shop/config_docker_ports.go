package shop

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
	"gopkg.in/yaml.v3"
)

// Keys for the docker.ports configuration. Each key names a host port that the
// generated compose.yaml publishes; the container-side port is fixed.
const (
	DockerPortWeb                     = "web"
	DockerPortWebAlt                  = "web_alt"
	DockerPortStorefrontWatcherAssets = "storefront_watcher_assets"
	DockerPortStorefrontWatcher       = "storefront_watcher"
	DockerPortAdminWatcher            = "admin_watcher"
	DockerPortAdminWatcherHMR         = "admin_watcher_hmr"
	DockerPortAdminer                 = "adminer"
	DockerPortMailerSMTP              = "mailer_smtp"
	DockerPortMailerWeb               = "mailer_web"
	DockerPortAMQPManagement          = "amqp_management"
	DockerPortAMQP                    = "amqp"
	DockerPortElasticsearch           = "elasticsearch"
	DockerPortS3                      = "s3"
	DockerPortS3Console               = "s3_console"
)

// ConfigDockerPorts maps a published-port key to the host port it binds, or to
// false to not publish the port at all.
type ConfigDockerPorts map[string]ConfigDockerPort

// ConfigDockerPort is a host port override. Its YAML value is either a port
// number or false to disable publishing the port.
type ConfigDockerPort int

// DockerPortDisabled marks a port that is not published to the host (the YAML
// value false).
const DockerPortDisabled ConfigDockerPort = -1

// Disabled reports whether the port should not be published to the host.
func (p ConfigDockerPort) Disabled() bool {
	return p == DockerPortDisabled
}

func (p *ConfigDockerPort) UnmarshalYAML(node *yaml.Node) error {
	var disabled bool
	if err := node.Decode(&disabled); err == nil {
		if disabled {
			return fmt.Errorf("docker.ports: %q is not a valid value, use a port number or false", node.Value)
		}
		*p = DockerPortDisabled
		return nil
	}

	var port int
	if err := node.Decode(&port); err != nil {
		return fmt.Errorf("docker.ports: expected a port number or false, got %q", node.Value)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("docker.ports: %d is not a valid port number", port)
	}

	*p = ConfigDockerPort(port)
	return nil
}

func (p ConfigDockerPort) MarshalYAML() (any, error) {
	if p.Disabled() {
		return false, nil
	}
	return int(p), nil
}

func (ConfigDockerPort) JSONSchema() *jsonschema.Schema {
	minimum := json.Number("1")
	maximum := json.Number("65535")
	return &jsonschema.Schema{
		Description: "Host port number, or false to not publish the port.",
		OneOf: []*jsonschema.Schema{
			{
				Type:    "integer",
				Minimum: minimum,
				Maximum: maximum,
			},
			{
				Const: false,
			},
		},
	}
}

func (ConfigDockerPorts) JSONSchemaExtend(s *jsonschema.Schema) {
	ports := []struct {
		key         string
		description string
	}{
		{DockerPortWeb, "Host port for the shop (Caddy). Defaults to 8000."},
		{DockerPortWebAlt, "Alternative HTTP host port of the web container. Defaults to 8080."},
		{DockerPortStorefrontWatcherAssets, "Host port for the storefront watcher assets. Defaults to 9999."},
		{DockerPortStorefrontWatcher, "Host port for the storefront watcher proxy. Defaults to 9998."},
		{DockerPortAdminWatcher, "Host port for the administration watcher (Vite). Defaults to 5173."},
		{DockerPortAdminWatcherHMR, "Host port for the administration watcher hot module reload. Defaults to 5773."},
		{DockerPortAdminer, "Host port for the Adminer UI. Defaults to 9080."},
		{DockerPortMailerSMTP, "Host port for the Mailpit SMTP endpoint. Defaults to 1025."},
		{DockerPortMailerWeb, "Host port for the Mailpit web UI. Defaults to 8025."},
		{DockerPortAMQPManagement, "Host port for the LavinMQ management UI. Defaults to 15672."},
		{DockerPortAMQP, "Host port for the AMQP endpoint. Defaults to 5672."},
		{DockerPortElasticsearch, "Host port for OpenSearch. Defaults to 9200."},
		{DockerPortS3, "Host port for the S3 API (RustFS). Defaults to 9000."},
		{DockerPortS3Console, "Host port for the RustFS console. Defaults to 9001."},
	}

	properties := orderedmap.New[string, *jsonschema.Schema]()
	for _, port := range ports {
		properties.Set(port.key, &jsonschema.Schema{
			Ref:         "#/$defs/ConfigDockerPort",
			Description: port.description,
		})
	}

	s.Properties = properties
	s.AdditionalProperties = jsonschema.FalseSchema
}

// DockerPorts returns the configured host-port overrides, nil when none are
// set. Nil-safe for a nil config.
func (c *Config) DockerPorts() ConfigDockerPorts {
	if c == nil || c.Docker == nil {
		return nil
	}
	return c.Docker.Ports
}

// SetDockerPortOverrides merges host-port overrides into c.Docker.Ports,
// creating the intermediate structs as needed.
func (c *Config) SetDockerPortOverrides(ports map[string]int) {
	if len(ports) == 0 {
		return
	}

	if c.Docker == nil {
		c.Docker = &ConfigDocker{}
	}

	if c.Docker.Ports == nil {
		c.Docker.Ports = ConfigDockerPorts{}
	}

	for key, port := range ports {
		c.Docker.Ports[key] = ConfigDockerPort(port)
	}
}
