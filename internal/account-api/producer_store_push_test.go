package account_api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func demoGeneralInfo() *ExtensionGeneralInformation {
	return &ExtensionGeneralInformation{
		Locales: []Locale{
			{Id: 1, Name: "de_DE"},
			{Id: 2, Name: "en_GB"},
		},
		DemoTypes: []StoreDemoType{
			{Id: 1, Name: "frontend", Description: "Frontend demo"},
			{Id: 2, Name: "backend", Description: "Backend demo"},
		},
	}
}

func TestBuildExtensionDemos(t *testing.T) {
	demos, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{
			Type:          "frontend",
			Link:          "https://demo.example.com",
			Localization:  "de_DE",
			LoginName:     "demo",
			LoginPassword: "secret",
		},
		{
			Type:         "backend",
			Link:         "https://demo.example.com/admin",
			Localization: "en_GB",
		},
	}, demoGeneralInfo())

	require.NoError(t, err)
	assert.Equal(t, []ExtensionDemo{
		{
			Type:          StoreDemoType{Id: 1, Name: "frontend", Description: "Frontend demo"},
			Link:          "https://demo.example.com",
			Localization:  Locale{Id: 1, Name: "de_DE"},
			LoginName:     "demo",
			LoginPassword: "secret",
		},
		{
			Type:         StoreDemoType{Id: 2, Name: "backend", Description: "Backend demo"},
			Link:         "https://demo.example.com/admin",
			Localization: Locale{Id: 2, Name: "en_GB"},
		},
	}, demos)
}

func TestBuildExtensionDemosEmptyList(t *testing.T) {
	demos, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{}, demoGeneralInfo())

	require.NoError(t, err)
	assert.Empty(t, demos)
	assert.NotNil(t, demos)
}

func TestBuildExtensionDemosUnknownType(t *testing.T) {
	_, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{Type: "storefront", Link: "https://demo.example.com", Localization: "de_DE"},
	}, demoGeneralInfo())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo shop type \"storefront\"")
	assert.Contains(t, err.Error(), "frontend, backend")
}

func TestBuildExtensionDemosUnknownLocalization(t *testing.T) {
	_, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{Type: "frontend", Link: "https://demo.example.com", Localization: "fr_FR"},
	}, demoGeneralInfo())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo shop localization \"fr_FR\"")
	assert.Contains(t, err.Error(), "de_DE, en_GB")
}

func TestUpdateStoreInfoKeepsDemosWhenNotConfigured(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	require.NoError(t, updateStoreInfo(ext, nil, &extension.Config{}, demoGeneralInfo()))
	assert.Len(t, ext.Demos, 1)
	assert.Equal(t, 42, ext.Demos[0].Id)
}

func TestUpdateStoreInfoReplacesDemos(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://old.example.com"},
		},
	}

	cfg := &extension.Config{}
	cfg.Store.DemoShops = &[]extension.ConfigStoreDemoShop{
		{Type: "backend", Link: "https://demo.example.com/admin", Localization: "en_GB"},
	}

	require.NoError(t, updateStoreInfo(ext, nil, cfg, demoGeneralInfo()))
	require.Len(t, ext.Demos, 1)
	assert.Equal(t, 0, ext.Demos[0].Id)
	assert.Equal(t, "backend", ext.Demos[0].Type.Name)
	assert.Equal(t, "https://demo.example.com/admin", ext.Demos[0].Link)
	assert.Equal(t, "en_GB", ext.Demos[0].Localization.Name)
}

func TestUpdateStoreInfoClearsDemos(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	cfg := &extension.Config{}
	cfg.Store.DemoShops = &[]extension.ConfigStoreDemoShop{}

	require.NoError(t, updateStoreInfo(ext, nil, cfg, demoGeneralInfo()))
	assert.Empty(t, ext.Demos)
}

func TestUpdateStoreInfoAppliesTranslations(t *testing.T) {
	var ext Extension
	require.NoError(t, json.Unmarshal([]byte(`{"infos":[{"locale":{"name":"de_DE"}},{"locale":{"name":"en_GB"}}]}`), &ext))

	deMetaTitle := "German title"
	enMetaTitle := "English title"
	deTags := []string{"tag-de"}
	enTags := []string{"tag-en"}

	cfg := &extension.Config{}
	cfg.Store.MetaTitle = extension.ConfigTranslated[string]{German: &deMetaTitle, English: &enMetaTitle}
	cfg.Store.Tags = extension.ConfigTranslated[[]string]{German: &deTags, English: &enTags}

	require.NoError(t, updateStoreInfo(&ext, nil, cfg, demoGeneralInfo()))

	assert.Equal(t, "German title", ext.Infos[0].MetaTitle)
	assert.Equal(t, "English title", ext.Infos[1].MetaTitle)
	assert.Equal(t, []StoreTag{{Name: "tag-de"}}, ext.Infos[0].Tags)
	assert.Equal(t, []StoreTag{{Name: "tag-en"}}, ext.Infos[1].Tags)
}

func TestGetTranslation(t *testing.T) {
	german := "de"
	english := "en"
	config := extension.ConfigTranslated[string]{German: &german, English: &english}

	assert.Equal(t, &german, getTranslation("de", config))
	assert.Equal(t, &english, getTranslation("en", config))
	assert.Nil(t, getTranslation("fr", config))
}

func TestParseInlineablePathPlainText(t *testing.T) {
	content, err := parseInlineablePath("plain text", "/does-not-matter")

	require.NoError(t, err)
	assert.Equal(t, "plain text", content)
}

func TestParseInlineablePathReadsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.html"), []byte("<p>Hello</p>"), 0o644))

	content, err := parseInlineablePath("file:description.html", dir)

	require.NoError(t, err)
	assert.Equal(t, "<p>Hello</p>", content)
}

func TestParseInlineablePathConvertsMarkdown(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("# Hello"), 0o644))

	content, err := parseInlineablePath("file:description.md", dir)

	require.NoError(t, err)
	assert.Contains(t, content, "Hello")
	assert.NotEqual(t, "# Hello", content)
}

func TestParseInlineablePathMissingFile(t *testing.T) {
	_, err := parseInlineablePath("file:missing.html", t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading file")
}
