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

// Directory is the set of known integrations.
type Directory struct {
	Integrations []Integration `json:"integrations"`
}

// Integration is a single directory entry. The json field names are a public
// contract; see CONTRACT.md.
type Integration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Repository  string `json:"repository"`
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

// List returns all skills, or only names in installed when installedOnly is set.
func (d *Directory) List(installed map[string]bool, installedOnly bool) []Integration {
	out := make([]Integration, 0, len(d.Integrations))
	for _, e := range d.Integrations {
		if !installedOnly || installed[e.Name] {
			out = append(out, e)
		}
	}
	return out
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
