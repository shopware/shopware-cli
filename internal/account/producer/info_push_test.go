package producer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
)

func demoGeneralInfo() *accountApi.ExtensionGeneralInformation {
	return &accountApi.ExtensionGeneralInformation{
		Locales: []accountApi.Locale{
			{Id: 1, Name: "de_DE"},
			{Id: 2, Name: "en_GB"},
		},
		DemoTypes: []accountApi.StoreDemoType{
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
	assert.Equal(t, []accountApi.ExtensionDemo{
		{
			Type:          accountApi.StoreDemoType{Id: 1, Name: "frontend", Description: "Frontend demo"},
			Link:          "https://demo.example.com",
			Localization:  accountApi.Locale{Id: 1, Name: "de_DE"},
			LoginName:     "demo",
			LoginPassword: "secret",
		},
		{
			Type:         accountApi.StoreDemoType{Id: 2, Name: "backend", Description: "Backend demo"},
			Link:         "https://demo.example.com/admin",
			Localization: accountApi.Locale{Id: 2, Name: "en_GB"},
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
	ext := &accountApi.Extension{
		Demos: []accountApi.ExtensionDemo{
			{Id: 42, Type: accountApi.StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	require.NoError(t, updateStoreInfo(ext, nil, &extension.Config{}, demoGeneralInfo()))
	assert.Len(t, ext.Demos, 1)
	assert.Equal(t, 42, ext.Demos[0].Id)
}

func TestUpdateStoreInfoReplacesDemos(t *testing.T) {
	ext := &accountApi.Extension{
		Demos: []accountApi.ExtensionDemo{
			{Id: 42, Type: accountApi.StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://old.example.com"},
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
	ext := &accountApi.Extension{
		Demos: []accountApi.ExtensionDemo{
			{Id: 42, Type: accountApi.StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	cfg := &extension.Config{}
	cfg.Store.DemoShops = &[]extension.ConfigStoreDemoShop{}

	require.NoError(t, updateStoreInfo(ext, nil, cfg, demoGeneralInfo()))
	assert.Empty(t, ext.Demos)
}

func TestPushStoreInfoWithoutConfig(t *testing.T) {
	storeExt := &accountApi.Extension{}
	storeExt.Id = 7

	api := &fakeStoreInfoPushAPI{
		extension:   storeExt,
		generalInfo: demoGeneralInfo(),
	}

	zipExt := &fakeExtension{
		name: "AcmePlugin",
		metadata: &extension.ExtensionMetadata{
			Label:       extension.ExtensionTranslated{German: "Acme DE", English: "Acme EN"},
			Description: extension.ExtensionTranslated{German: "Beschreibung", English: "Description"},
		},
	}

	require.NoError(t, PushStoreInfo(t.Context(), api, zipExt))

	assert.Same(t, storeExt, api.updatedExtension)
	assert.Empty(t, api.updatedIconPath)
}

func TestPushStoreInfoUpdatesIcon(t *testing.T) {
	storeExt := &accountApi.Extension{}
	storeExt.Id = 7

	api := &fakeStoreInfoPushAPI{
		extension:   storeExt,
		generalInfo: demoGeneralInfo(),
	}

	icon := "src/Resources/store/icon.png"
	cfg := &extension.Config{}
	cfg.Store.Icon = &icon

	zipExt := &fakeExtension{
		name:     "AcmePlugin",
		path:     "/tmp/AcmePlugin",
		metadata: &extension.ExtensionMetadata{},
		config:   cfg,
	}

	require.NoError(t, PushStoreInfo(t.Context(), api, zipExt))

	assert.Equal(t, "/tmp/AcmePlugin/src/Resources/store/icon.png", api.updatedIconPath)
	assert.Same(t, storeExt, api.updatedExtension)
}

func TestParseInlineablePath(t *testing.T) {
	t.Run("plain text is passed through", func(t *testing.T) {
		result, err := parseInlineablePath("Just a description", "/does/not/matter")
		require.NoError(t, err)
		assert.Equal(t, "Just a description", result)
	})

	t.Run("file reference is inlined", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "description.html"), []byte("<p>Hello</p>"), 0o644))

		result, err := parseInlineablePath("file:description.html", dir)
		require.NoError(t, err)
		assert.Equal(t, "<p>Hello</p>", result)
	})

	t.Run("markdown file is converted to html", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("# Hello"), 0o644))

		result, err := parseInlineablePath("file:description.md", dir)
		require.NoError(t, err)
		assert.Contains(t, result, "Hello")
		assert.Contains(t, result, `class="h1"`)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		_, err := parseInlineablePath("file:missing.html", t.TempDir())
		require.Error(t, err)
	})
}
