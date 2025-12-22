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
