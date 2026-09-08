package testhelper

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type manifestMeta struct {
	Name          string   `xml:"name"`
	Labels        []string `xml:"label"`
	Descriptions  []string `xml:"description"`
	Compatibility string   `xml:"compatibility"`
	Author        string   `xml:"author"`
	Copyright     string   `xml:"copyright"`
	Version       string   `xml:"version"`
	License       string   `xml:"license"`
	Icon          string   `xml:"icon"`
}

type manifestDoc struct {
	Meta  manifestMeta `xml:"meta"`
	Setup *struct {
		Secret string `xml:"secret"`
	} `xml:"setup"`
}

func decodeManifest(t *testing.T, s string) manifestDoc {
	t.Helper()
	var doc manifestDoc
	require.NoError(t, xml.Unmarshal([]byte(s), &doc))
	return doc
}

func TestNewAppManifestRendersCompleteMeta(t *testing.T) {
	doc := decodeManifest(t, NewAppManifest("MyExampleApp").String())

	assert.Equal(t, "MyExampleApp", doc.Meta.Name)
	// The default language entry must come before translations.
	assert.Equal(t, []string{"Label", "Name"}, doc.Meta.Labels)
	assert.Equal(t, []string{"A description", "Eine Beschreibung"}, doc.Meta.Descriptions)
	assert.Equal(t, "Your Company Ltd.", doc.Meta.Author)
	assert.Equal(t, "(c) by Your Company Ltd.", doc.Meta.Copyright)
	assert.Equal(t, "1.0.0", doc.Meta.Version)
	assert.Equal(t, "MIT", doc.Meta.License)
	assert.Empty(t, doc.Meta.Compatibility)
	assert.Empty(t, doc.Meta.Icon)
	assert.Nil(t, doc.Setup)
}

func TestAppManifestOmitsClearedFields(t *testing.T) {
	m := NewAppManifest("MyExampleApp")
	m.License = ""

	rendered := m.String()
	assert.NotContains(t, rendered, "<license>")
	assert.Empty(t, decodeManifest(t, rendered).Meta.License)
}

func TestAppManifestOptionalElements(t *testing.T) {
	m := NewAppManifest("MyExampleApp")
	m.Compatibility = "~6.5.0"
	m.Icon = "app.png"
	m.SetupSecret = "foo"

	doc := decodeManifest(t, m.String())
	assert.Equal(t, "~6.5.0", doc.Meta.Compatibility)
	assert.Equal(t, "app.png", doc.Meta.Icon)
	require.NotNil(t, doc.Setup)
	assert.Equal(t, "foo", doc.Setup.Secret)
}
