package validation

import (
	"fmt"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"
)

// CheckResult represents a validation result
type CheckResult struct {
	// The path to the file that was checked
	Path string `json:"path"`
	// The line number of the issue
	Line    int    `json:"line"`
	Message string `json:"message"`
	// The severity of the issue
	Severity string `json:"severity"`

	Identifier string `json:"identifier"`
}

// ToolConfigIgnore represents a configuration item to ignore during validation
type ToolConfigIgnore struct {
	Identifier string `yaml:"identifier"`
	Path       string `yaml:"path,omitempty"`
	Message    string `yaml:"message,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for ToolConfigIgnore
func (c *ToolConfigIgnore) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Identifier = value.Value
		return nil
	}

	type objectFormat struct {
		Identifier string `yaml:"identifier"`
		Path       string `yaml:"path,omitempty"`
		Message    string `yaml:"message,omitempty"`
	}
	var obj objectFormat
	if err := value.Decode(&obj); err != nil {
		return fmt.Errorf("failed to decode ToolConfigIgnore: %w", err)
	}

	c.Identifier = obj.Identifier
	c.Path = obj.Path
	c.Message = obj.Message

	return nil
}

// JSONSchema generates the JSON schema for ToolConfigIgnore
func (c ToolConfigIgnore) JSONSchema() *jsonschema.Schema {
	ordMap := orderedmap.New[string, *jsonschema.Schema]()

	ordMap.Set("identifier", &jsonschema.Schema{
		Type:        "string",
		Description: "The identifier of the item to ignore.",
	})

	ordMap.Set("path", &jsonschema.Schema{
		Type:        "string",
		Description: "The path of the item to ignore.",
	})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:       "object",
				Properties: ordMap,
			},
			{
				Type: "string",
			},
		},
	}
}

// Check interface for validation checking
type Check interface {
	AddResult(CheckResult)
	RemoveByIdentifier([]ToolConfigIgnore) Check
	GetResults() []CheckResult
	HasErrors() bool
}

// Severity constants
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)
