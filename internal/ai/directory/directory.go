// Package directory provides the embedded, curated directory of Shopware AI
// integrations that backs the "shopware-cli ai list" and "ai info" commands.
//
// The directory is a single YAML manifest (integrations.yaml) embedded into the
// binary. It requires no network access and is the source of truth for
// integration discovery. The frozen field contract lives in CONTRACT.md.
package directory

import (
	_ "embed"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed integrations.yaml
var manifestYAML []byte

// Type is the kind of integration. Only "skill" is valid today; "mcp" is
// reserved for a later increment (shopware/shopware-cli#1336).
type Type string

const (
	TypeSkill Type = "skill"
)

// DeliveryKind describes how an integration is delivered. "project" is reserved
// for a later increment and must not be renamed when added.
type DeliveryKind string

const (
	DeliveryBundled DeliveryKind = "bundled"
	DeliveryGit     DeliveryKind = "git"
)

// Status is the lifecycle state of an integration.
type Status string

const (
	StatusActive     Status = "active"
	StatusComingSoon Status = "coming-soon"
	StatusDeprecated Status = "deprecated"
)

// Directory is the parsed manifest.
type Directory struct {
	Version      int           `yaml:"version" json:"version" jsonschema:"required"`
	Integrations []Integration `yaml:"integrations" json:"integrations" jsonschema:"required"`
}

// Integration is a single directory entry.
//
// The struct carries both yaml (manifest) and json (public output) tags. The
// json field names are a public contract; see CONTRACT.md. Fields tagged
// yaml:"-" are computed for output only and are never read from the manifest.
// The Internal field is tagged json:"-" and must never appear in any output.
type Integration struct {
	Name        string `yaml:"name" json:"name" jsonschema:"required"`
	DisplayName string `yaml:"display_name" json:"displayName" jsonschema:"required"`
	Type        Type   `yaml:"type" json:"type" jsonschema:"required,enum=skill"`
	Provider    string `yaml:"provider" json:"provider" jsonschema:"required"`
	Description string `yaml:"description" json:"description" jsonschema:"required"`
	Status      Status `yaml:"status" json:"status" jsonschema:"required,enum=active,enum=coming-soon,enum=deprecated"`

	// Available and AvailabilityReason are computed at output time (step 9 /
	// project detection in #1336), not stored in the manifest.
	Available          bool   `yaml:"-" json:"available"`
	AvailabilityReason string `yaml:"-" json:"availabilityReason,omitempty"`

	Documentation string         `yaml:"documentation" json:"documentation" jsonschema:"required,format=uri"`
	Delivery      Delivery       `yaml:"delivery" json:"delivery" jsonschema:"required"`
	Compatibility *Compatibility `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`

	// Internal holds maintainer-facing metadata. It is a legal, optional manifest
	// field (so the schema allows it), but json:"-" keeps it out of every
	// user-facing output.
	Internal *Internal `yaml:"internal,omitempty" json:"-"`
}

// Delivery describes how an integration is delivered. Repository is required
// when Kind == DeliveryGit (enforced by Validate). A future "project" kind adds
// an endpoint field here without renaming existing fields.
type Delivery struct {
	Kind       DeliveryKind `yaml:"kind" json:"kind" jsonschema:"required,enum=bundled,enum=git"`
	Repository string       `yaml:"repository,omitempty" json:"repository,omitempty" jsonschema:"format=uri"`
}

// Compatibility describes how compatibility is determined for an entry.
type Compatibility struct {
	Source string `yaml:"source" json:"source" jsonschema:"required,enum=owner"`
}

// Internal is maintainer-only metadata, never exposed in user-facing output.
type Internal struct {
	Maintainer string `yaml:"maintainer" json:"-" jsonschema:"required"`
}

var (
	loadOnce sync.Once
	loaded   struct {
		dir *Directory
		err error
	}
)

// Get returns the integration with the given name and true, or nil and false
// when no entry matches.
func (d *Directory) Get(name string) (*Integration, bool) {
	for i := range d.Integrations {
		if d.Integrations[i].Name == name {
			return &d.Integrations[i], true
		}
	}

	return nil, false
}

// Load parses the embedded manifest once and caches the result. Subsequent
// calls return the cached directory. It does not validate the contents; use
// Validate for that.
func Load() (*Directory, error) {
	loadOnce.Do(func() {
		var d Directory
		if err := yaml.Unmarshal(manifestYAML, &d); err != nil {
			loaded.err = fmt.Errorf("parse embedded ai directory manifest: %w", err)
			return
		}
		loaded.dir = &d
	})
	return loaded.dir, loaded.err
}
