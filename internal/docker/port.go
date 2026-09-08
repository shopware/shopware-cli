package docker

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
)

// Port is a host-port override. Its YAML value is either a port number or
// false to not publish the port at all.
type Port int

// PortDisabled marks a port that is not published to the host (the YAML value
// false).
const PortDisabled Port = -1

// Disabled reports whether the port should not be published to the host.
func (p Port) Disabled() bool {
	return p == PortDisabled
}

func (p *Port) UnmarshalYAML(node *yaml.Node) error {
	var disabled bool
	if err := node.Decode(&disabled); err == nil {
		if disabled {
			return fmt.Errorf("%q is not a valid port value, use a port number or false", node.Value)
		}
		*p = PortDisabled
		return nil
	}

	var port int
	if err := node.Decode(&port); err != nil {
		return fmt.Errorf("expected a port number or false, got %q", node.Value)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%d is not a valid port number", port)
	}

	*p = Port(port)
	return nil
}

func (p Port) MarshalYAML() (any, error) {
	if p.Disabled() {
		return false, nil
	}
	return int(p), nil
}

func (Port) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Description: "Host port number, or false to not publish the port.",
		OneOf: []*jsonschema.Schema{
			{
				Type:    "integer",
				Minimum: json.Number("1"),
				Maximum: json.Number("65535"),
			},
			{
				Const: false,
			},
		},
	}
}

// Ports maps a service's endpoint name to the host port it binds, or to
// PortDisabled to not publish the endpoint at all.
type Ports map[string]Port

// PortOverride pins one service endpoint to a host port.
type PortOverride struct {
	Service  string
	Endpoint string
	HostPort int
}
