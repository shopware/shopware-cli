package extension

import (
	"errors"
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/shop"
)

// ConfigSystemConfigRule is one system_config.configuration_key to anonymize.
// A string entry omits that row from the dump. An object sets omit: true, or
// a value. A value that starts with faker. is generated per row. Any other
// value is stored as {"_value": <value>}.
type ConfigSystemConfigRule struct {
	Key      string
	Omit     bool
	HasOmit  bool
	HasValue bool
	Value    any
}

// UnmarshalYAML accepts a configuration key string, which omits the row, or
// an object with key plus omit or value.
func (r *ConfigSystemConfigRule) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.Key = strings.TrimSpace(value.Value)
		r.Omit = true
		r.HasOmit = true
		return nil
	}

	if value.Kind != yaml.MappingNode {
		return errors.New("system_config entry must be a configuration key or an object with key")
	}

	for i := 0; i+1 < len(value.Content); i += 2 {
		field := value.Content[i].Value
		raw := value.Content[i+1]
		switch field {
		case "key":
			if err := raw.Decode(&r.Key); err != nil {
				return err
			}
			r.Key = strings.TrimSpace(r.Key)
		case "omit":
			if err := raw.Decode(&r.Omit); err != nil {
				return err
			}
			r.HasOmit = true
		case "value":
			if err := raw.Decode(&r.Value); err != nil {
				return err
			}
			r.HasValue = true
		default:
			return fmt.Errorf("unknown system_config field %q", field)
		}
	}

	return nil
}

// JSONSchema documents the string shorthand and the object form.
func (ConfigSystemConfigRule) JSONSchema() *jsonschema.Schema {
	object := orderedmap.New[string, *jsonschema.Schema]()
	object.Set("key", &jsonschema.Schema{
		Type:        "string",
		Description: "system_config.configuration_key, such as SwagPayPal.settings.clientSecret.",
	})
	object.Set("omit", &jsonschema.Schema{
		Type:        "boolean",
		Description: "When true, the dump skips rows with this configuration_key.",
	})
	object.Set("value", &jsonschema.Schema{
		Description: "Replacement stored as {\"_value\": value}. A string starting with faker. is generated per row, for example faker.Internet.Email().",
	})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:        "string",
				Description: "configuration_key to omit from the dump.",
			},
			{
				Type:       "object",
				Properties: object,
			},
		},
	}
}

// DumpRule converts the config entry into a dump rule.
func (r ConfigSystemConfigRule) DumpRule() (shop.SystemConfigRule, error) {
	if r.HasOmit && !r.Omit && !r.HasValue {
		return shop.SystemConfigRule{}, fmt.Errorf("system_config key %q sets omit: false without a value", r.Key)
	}

	return shop.NewSystemConfigRule(r.Key, r.Omit, r.HasValue, r.Value)
}
