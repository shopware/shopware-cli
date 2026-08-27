package directory

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
)

// GenerateSchemaJSON reflects the manifest types (Directory) into an indented
// JSON Schema document. It is the single source of truth used both to write
// schema.json (scripts/schema.go) and to assert in a test that the committed
// schema.json stays in sync with the Go types.
func GenerateSchemaJSON() ([]byte, error) {
	r := &jsonschema.Reflector{
		FieldNameTag:               "yaml",
		RequiredFromJSONSchemaTags: true,
	}

	schema := r.Reflect(&Directory{})

	// Encode the delivery invariants that Validate enforces (see CONTRACT.md) so
	// external schema consumers reject the same manifests the CLI does:
	// repository is required for git and forbidden for any other kind.
	if delivery, ok := schema.Definitions["Delivery"]; ok {
		delivery.AllOf = []*jsonschema.Schema{
			{
				If:   deliveryKindIs(string(DeliveryGit)),
				Then: &jsonschema.Schema{Required: []string{"repository"}},
			},
			{
				If:   deliveryKindIs(string(DeliveryGit)),
				Else: &jsonschema.Schema{Not: &jsonschema.Schema{Required: []string{"repository"}}},
			},
		}
	}

	return json.MarshalIndent(schema, "", "  ")
}

// deliveryKindIs builds a schema that matches a Delivery whose kind equals v.
func deliveryKindIs(v string) *jsonschema.Schema {
	props := orderedmap.New[string, *jsonschema.Schema]()
	props.Set("kind", &jsonschema.Schema{Const: v})

	return &jsonschema.Schema{
		Required:   []string{"kind"},
		Properties: props,
	}
}
