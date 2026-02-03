package extension

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertPlugin(t *testing.T) {
	plugin := PlatformPlugin{
		path:   t.TempDir(),
		config: &Config{},
		Composer: PlatformComposerJson{
			Extra: platformComposerJsonExtra{
				ShopwarePluginClass: "FroshTools\\FroshTools",
			},
		},
	}

	assetSource := ConvertExtensionsToSources(getTestContext(), []Extension{plugin})

	assert.Len(t, assetSource, 1)
	froshTools := assetSource[0]

	assert.Equal(t, "FroshTools", froshTools.Name)
	assert.Equal(t, filepath.Join(plugin.path, "src"), froshTools.Path)
}

func TestConvertApp(t *testing.T) {
	app := App{
		path:   t.TempDir(),
		config: &Config{},
		manifest: Manifest{
			Meta: Meta{
				Name: "TestApp",
			},
		},
	}

	assetSource := ConvertExtensionsToSources(getTestContext(), []Extension{app})

	assert.Len(t, assetSource, 1)
	froshTools := assetSource[0]

	assert.Equal(t, "TestApp", froshTools.Name)
	assert.Equal(t, app.path, froshTools.Path)
}

func TestConvertExtraBundlesOfConfig(t *testing.T) {
	app := App{
		path: t.TempDir(),
		manifest: Manifest{
			Meta: Meta{
				Name: "TestApp",
			},
		},
		config: &Config{
			Build: ConfigBuild{
				ExtraBundles: []ConfigExtraBundle{
					{
						Path: "src/Fooo",
					},
				},
			},
		},
	}

	assetSource := ConvertExtensionsToSources(getTestContext(), []Extension{app})

	assert.Len(t, assetSource, 1)
	sourceOne := assetSource[0]

	assert.Equal(t, "TestApp", sourceOne.Name)
	assert.Equal(t, app.path, sourceOne.Path)
}

func TestConvertExtraBundlesOfConfigWithOverride(t *testing.T) {
	app := App{
		path: t.TempDir(),
		manifest: Manifest{
			Meta: Meta{
				Name: "TestApp",
			},
		},
		config: &Config{
			Build: ConfigBuild{
				ExtraBundles: []ConfigExtraBundle{
					{
						Name: "Bla",
						Path: "src/Fooo",
					},
				},
			},
		},
	}

	assetSource := ConvertExtensionsToSources(getTestContext(), []Extension{app})

	assert.Len(t, assetSource, 1)
	sourceOne := assetSource[0]

	assert.Equal(t, "TestApp", sourceOne.Name)
	assert.Equal(t, app.path, sourceOne.Path)
}

// Tests for ExtensionAssetConfig methods

func TestExtensionAssetConfig_Has(t *testing.T) {
	config := ExtensionAssetConfig{
		"TestPlugin": &ExtensionAssetConfigEntry{},
	}

	assert.True(t, config.Has("TestPlugin"))
	assert.False(t, config.Has("NonExistent"))
}

func TestExtensionAssetConfig_RequiresShopwareRepository(t *testing.T) {
	entryPath := "main.js"

	t.Run("returns false for empty config", func(t *testing.T) {
		config := ExtensionAssetConfig{}
		assert.False(t, config.RequiresShopwareRepository())
	})

	t.Run("returns false when all extensions use esbuild", func(t *testing.T) {
		config := ExtensionAssetConfig{
			"TestPlugin": &ExtensionAssetConfigEntry{
				Administration:             ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
				EnableESBuildForAdmin:      true,
				Storefront:                 ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
				EnableESBuildForStorefront: true,
			},
		}
		assert.False(t, config.RequiresShopwareRepository())
	})

	t.Run("returns true when admin extension needs webpack", func(t *testing.T) {
		config := ExtensionAssetConfig{
			"TestPlugin": &ExtensionAssetConfigEntry{
				Administration:        ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
				EnableESBuildForAdmin: false,
			},
		}
		assert.True(t, config.RequiresShopwareRepository())
	})

	t.Run("returns true when storefront extension needs webpack", func(t *testing.T) {
		config := ExtensionAssetConfig{
			"TestPlugin": &ExtensionAssetConfigEntry{
				Storefront:                 ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
				EnableESBuildForStorefront: false,
			},
		}
		assert.True(t, config.RequiresShopwareRepository())
	})
}

func TestExtensionAssetConfig_FilterByAdmin(t *testing.T) {
	entryPath := "main.js"

	config := ExtensionAssetConfig{
		"WithAdmin": &ExtensionAssetConfigEntry{
			Administration: ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
		},
		"WithStorefront": &ExtensionAssetConfigEntry{
			Storefront: ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
		},
		"WithBoth": &ExtensionAssetConfigEntry{
			Administration: ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
			Storefront:     ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
		},
	}

	filtered := config.FilterByAdmin()

	assert.Len(t, filtered, 2)
	assert.True(t, filtered.Has("WithAdmin"))
	assert.True(t, filtered.Has("WithBoth"))
	assert.False(t, filtered.Has("WithStorefront"))
}

func TestExtensionAssetConfig_FilterByAdminAndEsBuild(t *testing.T) {
	entryPath := "main.js"

	config := ExtensionAssetConfig{
		"EsBuildEnabled": &ExtensionAssetConfigEntry{
			Administration:        ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
			EnableESBuildForAdmin: true,
		},
		"EsBuildDisabled": &ExtensionAssetConfigEntry{
			Administration:        ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
			EnableESBuildForAdmin: false,
		},
	}

	t.Run("filters esbuild enabled", func(t *testing.T) {
		filtered := config.FilterByAdminAndEsBuild(true)
		assert.Len(t, filtered, 1)
		assert.True(t, filtered.Has("EsBuildEnabled"))
	})

	t.Run("filters esbuild disabled", func(t *testing.T) {
		filtered := config.FilterByAdminAndEsBuild(false)
		assert.Len(t, filtered, 1)
		assert.True(t, filtered.Has("EsBuildDisabled"))
	})
}

func TestExtensionAssetConfig_FilterByStorefrontAndEsBuild(t *testing.T) {
	entryPath := "main.js"

	config := ExtensionAssetConfig{
		"EsBuildEnabled": &ExtensionAssetConfigEntry{
			Storefront:                 ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
			EnableESBuildForStorefront: true,
		},
		"EsBuildDisabled": &ExtensionAssetConfigEntry{
			Storefront:                 ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
			EnableESBuildForStorefront: false,
		},
	}

	t.Run("filters esbuild enabled", func(t *testing.T) {
		filtered := config.FilterByStorefrontAndEsBuild(true)
		assert.Len(t, filtered, 1)
		assert.True(t, filtered.Has("EsBuildEnabled"))
	})

	t.Run("filters esbuild disabled", func(t *testing.T) {
		filtered := config.FilterByStorefrontAndEsBuild(false)
		assert.Len(t, filtered, 1)
		assert.True(t, filtered.Has("EsBuildDisabled"))
	})
}

func TestExtensionAssetConfig_Only(t *testing.T) {
	config := ExtensionAssetConfig{
		"Plugin1": &ExtensionAssetConfigEntry{},
		"Plugin2": &ExtensionAssetConfigEntry{},
		"Plugin3": &ExtensionAssetConfigEntry{},
	}

	filtered := config.Only([]string{"Plugin1", "Plugin3"})

	assert.Len(t, filtered, 2)
	assert.True(t, filtered.Has("Plugin1"))
	assert.True(t, filtered.Has("Plugin3"))
	assert.False(t, filtered.Has("Plugin2"))
}

func TestExtensionAssetConfig_Not(t *testing.T) {
	config := ExtensionAssetConfig{
		"Plugin1": &ExtensionAssetConfigEntry{},
		"Plugin2": &ExtensionAssetConfigEntry{},
		"Plugin3": &ExtensionAssetConfigEntry{},
	}

	filtered := config.Not([]string{"Plugin2"})

	assert.Len(t, filtered, 2)
	assert.True(t, filtered.Has("Plugin1"))
	assert.True(t, filtered.Has("Plugin3"))
	assert.False(t, filtered.Has("Plugin2"))
}

func TestExtensionAssetConfigEntry_RequiresBuild(t *testing.T) {
	entryPath := "main.js"

	t.Run("returns false when no entries", func(t *testing.T) {
		entry := &ExtensionAssetConfigEntry{}
		assert.False(t, entry.RequiresBuild())
	})

	t.Run("returns true when admin entry exists", func(t *testing.T) {
		entry := &ExtensionAssetConfigEntry{
			Administration: ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
		}
		assert.True(t, entry.RequiresBuild())
	})

	t.Run("returns true when storefront entry exists", func(t *testing.T) {
		entry := &ExtensionAssetConfigEntry{
			Storefront: ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
		}
		assert.True(t, entry.RequiresBuild())
	})

	t.Run("returns true when both entries exist", func(t *testing.T) {
		entry := &ExtensionAssetConfigEntry{
			Administration: ExtensionAssetConfigAdmin{EntryFilePath: &entryPath},
			Storefront:     ExtensionAssetConfigStorefront{EntryFilePath: &entryPath},
		}
		assert.True(t, entry.RequiresBuild())
	})
}
