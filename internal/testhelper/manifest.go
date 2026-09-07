package testhelper

import (
	"encoding/xml"
	"sort"
	"strings"
)

// AppManifest describes a manifest.xml fixture for a Shopware app. Zero-valued
// fields are omitted from the output, so validator tests can express a
// manifest missing exactly one required element by clearing that field on the
// value NewAppManifest returns.
type AppManifest struct {
	Name          string
	Label         map[string]string // lang -> text; "" is the default language
	Description   map[string]string // lang -> text; "" is the default language
	Compatibility string
	Author        string
	Copyright     string
	Version       string
	License       string
	Icon          string
	SetupSecret   string
}

// NewAppManifest returns a complete, valid app manifest with placeholder
// metadata. Tests clear or override individual fields to build variants.
func NewAppManifest(name string) AppManifest {
	return AppManifest{
		Name:        name,
		Label:       map[string]string{"": "Label", "de-DE": "Name"},
		Description: map[string]string{"": "A description", "de-DE": "Eine Beschreibung"},
		Author:      "Your Company Ltd.",
		Copyright:   "(c) by Your Company Ltd.",
		Version:     "1.0.0",
		License:     "MIT",
	}
}

// String renders the manifest as XML.
func (m AppManifest) String() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">` + "\n")
	b.WriteString("\t<meta>\n")

	writeElem := func(name, lang, value string) {
		if value == "" {
			return
		}
		b.WriteString("\t\t<")
		b.WriteString(name)
		if lang != "" {
			b.WriteString(` lang="`)
			b.WriteString(lang)
			b.WriteString(`"`)
		}
		b.WriteString(">")
		_ = xml.EscapeText(&b, []byte(value))
		b.WriteString("</")
		b.WriteString(name)
		b.WriteString(">\n")
	}
	writeLocalized := func(name string, values map[string]string) {
		langs := make([]string, 0, len(values))
		for lang := range values {
			langs = append(langs, lang)
		}
		sort.Strings(langs) // "" (default language) sorts first
		for _, lang := range langs {
			writeElem(name, lang, values[lang])
		}
	}

	writeElem("name", "", m.Name)
	writeLocalized("label", m.Label)
	writeLocalized("description", m.Description)
	writeElem("compatibility", "", m.Compatibility)
	writeElem("author", "", m.Author)
	writeElem("copyright", "", m.Copyright)
	writeElem("version", "", m.Version)
	writeElem("license", "", m.License)
	writeElem("icon", "", m.Icon)

	b.WriteString("\t</meta>\n")
	if m.SetupSecret != "" {
		b.WriteString("\t<setup>\n\t\t<secret>")
		_ = xml.EscapeText(&b, []byte(m.SetupSecret))
		b.WriteString("</secret>\n\t</setup>\n")
	}
	b.WriteString("</manifest>")
	return b.String()
}
