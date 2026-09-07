package docker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
)

// ValidateSettings rejects services, endpoints and types the catalog does not
// know, so a typo in docker.services fails at config read time instead of
// being silently ignored.
func ValidateSettings(s Settings) error {
	for _, name := range sortedKeys(s) {
		svc := s[name]
		def := byName(name)
		if def == nil || !def.configurable() {
			return fmt.Errorf("docker.services.%s: unknown service, valid services: %s", name, strings.Join(configurableServiceNames(), ", "))
		}
		if svc == nil {
			continue
		}

		if svc.Type != "" || svc.Version != "" {
			if len(def.Variants) == 0 {
				return fmt.Errorf("docker.services.%s: type and version are not configurable for this service", name)
			}
			if svc.Type != "" && def.variantNamed(svc.Type) == nil {
				return fmt.Errorf("docker.services.%s.type: unknown value %q, valid values: %s", name, svc.Type, strings.Join(def.variantNames(), ", "))
			}
		}

		for _, endpointName := range sortedKeys(svc.Ports) {
			if def.endpointNamed(endpointName) == nil {
				return fmt.Errorf("docker.services.%s.ports.%s: unknown port, valid ports: %s", name, endpointName, strings.Join(endpointNames(*def), ", "))
			}
		}
	}

	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func configurableServiceNames() []string {
	var names []string
	for _, svc := range configurableServices() {
		names = append(names, svc.Name)
	}
	return names
}

func endpointNames(svc service) []string {
	var names []string
	for _, ep := range svc.publishedEndpoints() {
		names = append(names, ep.Name)
	}
	return names
}

// SettingsSchema returns the JSON schema of docker.services: one object per
// configurable catalog service, each allowing only the keys that apply to it.
// Port values reference the "Port" definition the config schema declares.
func SettingsSchema() *jsonschema.Schema {
	services := orderedmap.New[string, *jsonschema.Schema]()
	for _, svc := range configurableServices() {
		services.Set(svc.Name, serviceSchema(svc))
	}

	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           services,
		AdditionalProperties: jsonschema.FalseSchema,
	}
}

func serviceSchema(svc service) *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()

	if len(svc.Variants) > 0 {
		names := make([]any, 0, len(svc.Variants))
		for _, v := range svc.Variants {
			names = append(names, v.Name)
		}
		properties.Set("type", &jsonschema.Schema{
			Type:        "string",
			Enum:        names,
			Description: fmt.Sprintf("Implementation to run. Defaults to %q.", svc.Variants[0].Name),
		})

		var defaults []string
		for _, v := range svc.Variants {
			defaults = append(defaults, fmt.Sprintf("%q for %s", v.DefaultTag, v.Name))
		}
		properties.Set("version", &jsonschema.Schema{
			Type:        "string",
			Description: "Image version (tag) to run. Defaults to " + strings.Join(defaults, ", ") + ".",
		})
	}

	if endpoints := svc.publishedEndpoints(); len(endpoints) > 0 {
		ports := orderedmap.New[string, *jsonschema.Schema]()
		for _, ep := range endpoints {
			description := fmt.Sprintf("Host port for %s. Defaults to %d.", ep.Label, ep.DefaultHostPort)
			if ep.DefaultHostPort == 0 {
				description = fmt.Sprintf("Host port for %s. Published on a random port by default.", ep.Label)
			}
			ports.Set(ep.Name, &jsonschema.Schema{
				Ref:         "#/$defs/Port",
				Description: description,
			})
		}
		properties.Set("ports", &jsonschema.Schema{
			Type:                 "object",
			Description:          "Host ports the service's endpoints are published on. false disables publishing an endpoint.",
			Properties:           ports,
			AdditionalProperties: jsonschema.FalseSchema,
		})
	}

	return &jsonschema.Schema{
		Type:                 "object",
		Description:          svc.Label + " service settings.",
		Properties:           properties,
		AdditionalProperties: jsonschema.FalseSchema,
	}
}
