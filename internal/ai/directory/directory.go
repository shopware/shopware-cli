// Package directory provides the curated directory of Shopware AI integrations
// that backs the "shopware-cli ai list" and "ai info" commands.
//
// There is no remote source, so the directory is hardwired in Go (see
// integrations.go). It requires no network access and is the source of truth
// for integration discovery. The field/output contract lives in CONTRACT.md.
package directory

import "fmt"

// Type is the kind of integration.
type Type string

const (
	TypeSkill Type = "skill"
	TypeMCP Type = "mcp"
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

// Directory is the set of known integrations.
type Directory struct {
	Version      int           `json:"version"`
	Integrations []Integration `json:"integrations"`
}

// Integration is a single directory entry. The json field names are a public
// contract; see CONTRACT.md. The Internal field is tagged json:"-" and must
// never appear in any output.
type Integration struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Type        Type   `json:"type"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
	Status      Status `json:"status"`

	// Available and AvailabilityReason are computed at output time (project
	// detection lands in #1336); they are not part of the stored entry.
	Available          bool   `json:"available"`
	AvailabilityReason string `json:"availabilityReason,omitempty"`

	Documentation string         `json:"documentation"`
	Delivery      Delivery       `json:"delivery"`
	Compatibility *Compatibility `json:"compatibility,omitempty"`

	// Internal holds maintainer-facing metadata. json:"-" keeps it out of every
	// user-facing output.
	Internal *Internal `json:"-"`
}

// Delivery describes how an integration is delivered. Repository is required
// when Kind == DeliveryGit (enforced by Validate). A future "project" kind adds
// an endpoint field here without renaming existing fields.
type Delivery struct {
	Kind       DeliveryKind `json:"kind"`
	Repository string       `json:"repository,omitempty"`
}

// Compatibility describes how compatibility is determined for an entry.
type Compatibility struct {
	Source string `json:"source"`
}

// Internal is maintainer-only metadata, never exposed in user-facing output.
type Internal struct {
	Maintainer string `json:"-"`
}

// Load returns the directory of known integrations.
func Load() *Directory {
	return &integrations
}

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

// ListOptions filters a directory listing.
type ListOptions struct {
	// Type, when set, keeps only entries of that type. It accepts any known type
	// identifier (see knownTypeFilters), including reserved ones that match no
	// entry yet.
	Type string
	// InstalledOnly keeps only entries whose name is in the installed set passed
	// to List.
	InstalledOnly bool
}

// knownTypeFilters are the type identifiers accepted by List. It includes "mcp",
// reserved for a future increment (#1336): it currently matches no entry, so a
// "mcp" filter returns an empty list rather than an error. Any other value is
// rejected.
var knownTypeFilters = map[string]bool{
	string(TypeSkill): true,
	string(TypeMCP):   true,
}

// List returns the integrations matching opts, with availability applied.
// installed is the set of integration names recorded as installed by the CLI;
// it is consulted only when opts.InstalledOnly is set.
func (d *Directory) List(installed map[string]bool, opts ListOptions) ([]Integration, error) {
	if opts.Type != "" && !knownTypeFilters[opts.Type] {
		return nil, fmt.Errorf("unknown type %q (allowed: skill, mcp)", opts.Type)
	}

	out := make([]Integration, 0, len(d.Integrations))
	for _, e := range d.Integrations {
		if opts.Type != "" && string(e.Type) != opts.Type {
			continue
		}
		if opts.InstalledOnly && !installed[e.Name] {
			continue
		}

		out = append(out, withAvailability(e))
	}

	return out, nil
}

// Info returns a single integration by name, with availability applied, or an
// error when no entry matches.
func (d *Directory) Info(name string) (Integration, error) {
	e, ok := d.Get(name)
	if !ok {
		return Integration{}, fmt.Errorf("unknown integration %q", name)
	}

	return withAvailability(*e), nil
}

// withAvailability fills the computed availability fields for an entry. v1 is
// static: a coming-soon entry is unavailable, everything else is available.
// Project-detected availability (Core MCP) arrives with #1336; there is no
// network access here.
func withAvailability(e Integration) Integration {
	if e.Status == StatusComingSoon {
		e.Available = false
		e.AvailabilityReason = "not yet released"

		return e
	}

	e.Available = true

	return e
}
