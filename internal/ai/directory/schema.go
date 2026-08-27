package directory

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
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

	return json.MarshalIndent(r.Reflect(&Directory{}), "", "  ")
}
