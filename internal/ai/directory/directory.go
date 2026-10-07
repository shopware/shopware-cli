// Package directory provides the curated directory of Shopware AI integrations
// that backs the "shopware-cli ai list" and "ai info" commands.
//
// There is no remote source, so the directory is hardwired in Go (see
// integrations.go). It requires no network access and is the source of truth
// for integration discovery. The field/output contract lives in CONTRACT.md.
package directory

import (
	"fmt"
	"strings"
)

// Type is the kind of integration.
type Type string

const (
	TypeSkill Type = "skill"
	TypeMCP   Type = "mcp"
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
	StatusDeprecated Status = "deprecated"
)

// Directory is the set of known integrations.
type Directory struct {
	Integrations []Integration `json:"integrations"`
}

// Integration is a single directory entry. The json field names are a public
// contract; see CONTRACT.md.
type Integration struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Type        Type   `json:"type"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
	Status      Status `json:"status"`

	Documentation string         `json:"documentation"`
	Delivery      Delivery       `json:"delivery"`
	Compatibility *Compatibility `json:"compatibility,omitempty"`
}

// Delivery describes how an integration is delivered.
type Delivery struct {
	Kind       DeliveryKind `json:"kind"`
	Repository string       `json:"repository,omitempty"`
}

// Compatibility describes how compatibility is determined for an entry.
type Compatibility struct {
	Source string `json:"source"`
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
	// Type, when set, keeps only entries of that type (see knownTypeFilters).
	Type string
}

// knownTypeFilters are the type identifiers List accepts. "mcp" is reserved and
// matches no entry yet (empty result, not an error); anything else is rejected.
var knownTypeFilters = map[string]bool{
	string(TypeSkill): true,
	string(TypeMCP):   true,
}

// List returns the integrations matching opts.
func (d *Directory) List(opts ListOptions) ([]Integration, error) {
	if opts.Type != "" && !knownTypeFilters[opts.Type] {
		return nil, fmt.Errorf("unknown type %q (allowed: skill, mcp)", opts.Type)
	}

	out := make([]Integration, 0, len(d.Integrations))
	for _, e := range d.Integrations {
		if opts.Type != "" && string(e.Type) != opts.Type {
			continue
		}

		out = append(out, e)
	}

	return out, nil
}

// Info returns a single integration by name, or an error when no entry matches.
func (d *Directory) Info(name string) (*Integration, error) {
	e, ok := d.Get(name)
	if !ok {
		names := make([]string, len(d.Integrations))
		for i, e := range d.Integrations {
			names[i] = e.Name
		}

		return nil, fmt.Errorf("unknown integration %q, available: %s", name, strings.Join(names, ", "))
	}

	return e, nil
}
