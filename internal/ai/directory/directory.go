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
	Version      int           `yaml:"version" json:"version"`
	Integrations []Integration `yaml:"integrations" json:"integrations"`
}

// Integration is a single directory entry.
//
// The struct carries both yaml (manifest) and json (public output) tags. The
// json field names are a public contract; see CONTRACT.md. Fields tagged
// yaml:"-" are computed for output only and are never read from the manifest.
// The Internal field is tagged json:"-" and must never appear in any output.
type Integration struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"display_name" json:"displayName"`
	Type        Type   `yaml:"type" json:"type"`
	Provider    string `yaml:"provider" json:"provider"`
	Description string `yaml:"description" json:"description"`
	Status      Status `yaml:"status" json:"status"`

	// Available and AvailabilityReason are computed at output time (step 9 /
	// project detection in #1336), not stored in the manifest.
	Available          bool   `yaml:"-" json:"available"`
	AvailabilityReason string `yaml:"-" json:"availabilityReason,omitempty"`

	Documentation string         `yaml:"documentation" json:"documentation"`
	Delivery      Delivery       `yaml:"delivery" json:"delivery"`
	Compatibility *Compatibility `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`

	// Internal holds maintainer-facing metadata. It is loaded from the manifest
	// but must never be emitted: json:"-" keeps it out of every output.
	Internal *Internal `yaml:"internal,omitempty" json:"-"`
}

// Delivery describes how an integration is delivered. Repository is required
// when Kind == DeliveryGit (enforced by Validate). A future "project" kind adds
// an endpoint field here without renaming existing fields.
type Delivery struct {
	Kind       DeliveryKind `yaml:"kind" json:"kind"`
	Repository string       `yaml:"repository,omitempty" json:"repository,omitempty"`
}

// Compatibility describes how compatibility is determined for an entry.
type Compatibility struct {
	Source string `yaml:"source" json:"source"`
}

// Internal is maintainer-only metadata, never exposed in user-facing output.
type Internal struct {
	Maintainer string `yaml:"maintainer" json:"-"`
}

var (
	loadOnce sync.Once
	loaded   *Directory
	loadErr  error
)

// Load parses the embedded manifest once and caches the result. Subsequent
// calls return the cached directory. It does not validate the contents; use
// Validate for that.
func Load() (*Directory, error) {
	loadOnce.Do(func() {
		var d Directory
		if err := yaml.Unmarshal(manifestYAML, &d); err != nil {
			loadErr = fmt.Errorf("parse embedded ai directory manifest: %w", err)
			return
		}
		loaded = &d
	})
	return loaded, loadErr
}
