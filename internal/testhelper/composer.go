package testhelper

import (
	"encoding/json"
	"maps"
	"strings"
)

// ComposerJSON describes a composer.json fixture. Zero-valued fields are left
// out of the rendered JSON, so partial and degenerate manifests (only a
// version, an empty object) stay expressible.
type ComposerJSON struct {
	Name        string
	Type        string
	Version     string
	License     string
	Description string
	Authors     []string // rendered as [{"name": ...}, ...]
	Require     map[string]string
	RequireDev  map[string]string
	PluginClass string            // extra.shopware-plugin-class
	Label       map[string]string // extra.label
	Extra       map[string]any    // additional extra entries, merged last
	Psr4        map[string]string // autoload.psr-4
}

// PluginComposer returns the composer.json for a Shopware platform plugin.
// class is the fully qualified bootstrap class (e.g. `Swag\Demo\Demo`); the
// en-GB label and the PSR-4 autoload prefix are derived from it. Callers can
// override any field before rendering.
func PluginComposer(name, version, class string) ComposerJSON {
	parts := strings.Split(class, `\`)
	c := ComposerJSON{
		Name:        name,
		Type:        "shopware-platform-plugin",
		Version:     version,
		Require:     map[string]string{"shopware/core": "~6.6.0"},
		PluginClass: class,
		Label:       map[string]string{"en-GB": parts[len(parts)-1]},
	}
	if len(parts) > 1 {
		c.Psr4 = map[string]string{strings.Join(parts[:len(parts)-1], `\`) + `\`: "src/"}
	}
	return c
}

// String renders the manifest as JSON.
func (c ComposerJSON) String() string {
	m := map[string]any{}
	if c.Name != "" {
		m["name"] = c.Name
	}
	if c.Type != "" {
		m["type"] = c.Type
	}
	if c.Version != "" {
		m["version"] = c.Version
	}
	if c.License != "" {
		m["license"] = c.License
	}
	if c.Description != "" {
		m["description"] = c.Description
	}
	if len(c.Authors) > 0 {
		authors := make([]map[string]string, 0, len(c.Authors))
		for _, name := range c.Authors {
			authors = append(authors, map[string]string{"name": name})
		}
		m["authors"] = authors
	}
	if len(c.Require) > 0 {
		m["require"] = c.Require
	}
	if len(c.RequireDev) > 0 {
		m["require-dev"] = c.RequireDev
	}
	extra := map[string]any{}
	if c.PluginClass != "" {
		extra["shopware-plugin-class"] = c.PluginClass
	}
	if len(c.Label) > 0 {
		extra["label"] = c.Label
	}
	maps.Copy(extra, c.Extra)
	if len(extra) > 0 {
		m["extra"] = extra
	}
	if len(c.Psr4) > 0 {
		m["autoload"] = map[string]any{"psr-4": c.Psr4}
	}
	return marshalIndent(m)
}

// LockPackage is one entry in a composer.lock packages list.
type LockPackage struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Type    string            `json:"type,omitempty"`
	Require map[string]string `json:"require,omitempty"`
}

// ComposerLock renders a composer.lock with the given packages and an empty
// packages-dev section.
func ComposerLock(packages ...LockPackage) string {
	if packages == nil {
		packages = []LockPackage{}
	}
	return marshalIndent(map[string]any{
		"packages":     packages,
		"packages-dev": []LockPackage{},
	})
}

// kebabCase converts a CamelCase plugin name to its kebab-case composer form:
// "FroshTools" -> "frosh-tools".
func kebabCase(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func marshalIndent(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err) // unreachable for the map/struct shapes above
	}
	return string(b)
}
