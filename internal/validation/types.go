package validation

import (
	"fmt"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
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

	// Tip provides additional context or suggestions to fix the issue
	Tip string `json:"tip,omitempty"`
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
	GetTarget() *Target
	GetToolRuns() []ToolRun
}

// Severity constants
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// TargetSource says how the Shopware baseline of a run was chosen.
type TargetSource string

const (
	// TargetSourceConstraint is the lowest release that satisfies the declared constraint.
	TargetSourceConstraint TargetSource = "constraint"
	// TargetSourceFallback is the default used when no release satisfies the constraint.
	TargetSourceFallback TargetSource = "fallback"
)

// Target is the Shopware version a validation run is evaluated against.
type Target struct {
	// Version is the concrete Shopware release, e.g. 6.7.14.2.
	Version string `json:"version"`
	// Source says how Version was chosen.
	Source TargetSource `json:"source"`
	// Constraint is the shopware/core requirement declared by the input.
	Constraint string `json:"constraint,omitempty"`
	// WithinConstraint reports whether Version satisfies Constraint.
	WithinConstraint bool `json:"within_constraint"`
}

// Describe renders the version together with the reason it was chosen.
func (t Target) Describe() string {
	if t.Source == TargetSourceFallback {
		return fmt.Sprintf("%s (fallback, no release matches %q)", t.Version, t.Constraint)
	}

	return fmt.Sprintf("%s (lowest release matching %q)", t.Version, t.Constraint)
}

// Tool run statuses.
const (
	ToolRunRan     = "ran"
	ToolRunSkipped = "skipped"
)

// ToolRun records how one tool took part in a validation run.
type ToolRun struct {
	Name string `json:"name"`
	// Status is ToolRunRan or ToolRunSkipped.
	Status string `json:"status"`
	// Baseline is the Shopware version the tool evaluated; empty for version-independent tools.
	Baseline string `json:"baseline,omitempty"`
	// Note explains a skip or a baseline that differs from the run's target.
	Note string `json:"note,omitempty"`
}

// NotEvaluated reports whether a version-aware tool did not cover the target version.
func (r ToolRun) NotEvaluated(target string) bool {
	if r.Baseline == "" {
		return false
	}

	return r.Status == ToolRunSkipped || r.Baseline != target
}
