package esbuild

import (
	"context"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"regexp"
	"testing"
)

func TestLoadRegularJSFileWithImportPrefixAdmin(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := createEntrypoint(tmpDir, "import {searchRankingPoint} from \"@administration/app/service/search-ranking.service\";")

	options := RemoteLoaderOptions{
		BaseUrl: "https://raw.githubusercontent.com/shopware/shopware/v6.5.7.3/src/Administration/Resources/app/administration/",
		Matchers: map[string]RemoteLoaderReplacer{
			"^@administration/": {Matching: regexp.MustCompile("^@administration/"), Replace: "src/"},
		},
	}

	build := api.BuildOptions{
		EntryPoints: []string{filePath},
		Plugins:     []api.Plugin{newRemoteLoaderPlugin(context.Background(), options)},
		Outfile:     "extension.js",
		Bundle:      true,
		Write:       false,
	}

	result := api.Build(build)
	assert.Len(t, result.Errors, 0)

	assert.Contains(t, string(result.OutputFiles[0].Contents), "HIGH_SEARCH_RANKING")
}

func createEntrypoint(tmpDir, content string) string {
	filePath := tmpDir + "/test.js"

	_ = os.WriteFile(path.Join(tmpDir, "test.js"), []byte(content), 0644)

	return filePath
}

func TestLoadRegularJSFileAdmin(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := createEntrypoint(tmpDir, "import {searchRankingPoint} from \"src/app/service/search-ranking.service\";")

	options := RemoteLoaderOptions{
		BaseUrl: "https://raw.githubusercontent.com/shopware/shopware/v6.5.7.3/src/Administration/Resources/app/administration/",
		Matchers: map[string]RemoteLoaderReplacer{
			"^src\\/": {Matching: regexp.MustCompile("^src/"), Replace: "src/"},
		},
	}

	build := api.BuildOptions{
		EntryPoints: []string{filePath},
		Plugins:     []api.Plugin{newRemoteLoaderPlugin(context.Background(), options)},
		Outfile:     "extension.js",
		Bundle:      true,
		Write:       false,
	}

	result := api.Build(build)
	assert.Len(t, result.Errors, 0)

	assert.Len(t, result.OutputFiles, 1)

	assert.Contains(t, string(result.OutputFiles[0].Contents), "HIGH_SEARCH_RANKING")
}

func TestLoadSCSSFromExternalSource(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := createEntrypoint(tmpDir, "import \"src/module/sw-cms/component/sw-cms-block/sw-cms-block.scss\";")

	options := RemoteLoaderOptions{
		BaseUrl: "https://raw.githubusercontent.com/shopware/shopware/v6.5.7.3/src/Administration/Resources/app/administration/",
		Matchers: map[string]RemoteLoaderReplacer{
			"^src\\/": {Matching: regexp.MustCompile("^src/"), Replace: "src/"},
		},
	}

	build := api.BuildOptions{
		EntryPoints: []string{filePath},
		Plugins:     []api.Plugin{newScssPlugin(context.Background()), newRemoteLoaderPlugin(context.Background(), options)},
		Outfile:     "extension.js",
		Bundle:      true,
		Write:       false,
	}

	result := api.Build(build)
	assert.Len(t, result.Errors, 0)
}

func TestLoadRegularJSFileStorefront(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := createEntrypoint(tmpDir, "import Plugin from \"src/plugin-system/plugin.class\"; class Foo extends Plugin {}; export {Foo}")

	options := RemoteLoaderOptions{
		BaseUrl: "https://raw.githubusercontent.com/shopware/shopware/v6.5.7.3/src/Storefront/Resources/app/storefront/",
		Matchers: map[string]RemoteLoaderReplacer{
			"^src\\/": {Matching: regexp.MustCompile("^src/"), Replace: "src/"},
		},
	}

	build := api.BuildOptions{
		EntryPoints: []string{filePath},
		Plugins:     []api.Plugin{newRemoteLoaderPlugin(context.Background(), options)},
		Outfile:     "extension.js",
		Bundle:      true,
		Write:       false,
	}

	result := api.Build(build)
	assert.Len(t, result.Errors, 0)

	assert.Len(t, result.OutputFiles, 1)

	assert.Contains(t, string(result.OutputFiles[0].Contents), "There is no valid element given")
}
