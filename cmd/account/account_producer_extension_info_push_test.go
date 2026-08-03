package account

import (
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
