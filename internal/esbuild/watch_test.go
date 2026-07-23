package esbuild

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuildEntrypointsReturnsContentAddressedPaths(t *testing.T) {
	dir := t.TempDir()
	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	require.NoError(t, os.MkdirAll(adminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("import picture from './picture.png'; import './style.css'; console.log('served-marker', picture)"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "style.css"), []byte(".served-marker { background-image: url('./picture.png'); }"), 0o644))
	pictureContents := []byte("picture-contents")
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "picture.png"), pictureContents, 0o644))

	options := NewAssetCompileOptionsAdmin("MyPlugin", dir)
	options.ProductionMode = false
	options.DisableSass = true

	esbuildContext, err := Context(getTestContext(), options)
	require.Nil(t, err)
	t.Cleanup(esbuildContext.Dispose)

	watchServer, serveErr := esbuildContext.Serve(api.ServeOptions{Host: "127.0.0.1"})
	require.NoError(t, serveErr)

	entrypoints, entrypointErr := RebuildEntrypoints(esbuildContext)

	require.NoError(t, entrypointErr)
	assert.Regexp(t, `^/my-plugin-[A-Z0-9]+\.js$`, entrypoints.JavaScript)
	assert.Regexp(t, `^/my-plugin-[A-Z0-9]+\.css$`, entrypoints.CSS)
	assert.NotEqual(t, "/extension.js", entrypoints.JavaScript)
	assert.NotEqual(t, "/extension.css", entrypoints.CSS)

	servedBodies := map[string]string{}

	for _, test := range []struct {
		name   string
		path   string
		marker string
	}{
		{name: "javascript", path: entrypoints.JavaScript, marker: "served-marker"},
		{name: "css", path: entrypoints.CSS, marker: ".served-marker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := readServeOutput(t, watchServer, test.path)
			servedBodies[test.name] = string(body)
			assert.Contains(t, servedBodies[test.name], test.marker)
		})
	}

	picturePathPattern := regexp.MustCompile(`\./picture-[A-Z0-9]+\.png`)
	javaScriptPicturePath := picturePathPattern.FindString(servedBodies["javascript"])
	cssPicturePath := picturePathPattern.FindString(servedBodies["css"])
	require.NotEmpty(t, javaScriptPicturePath)
	assert.Equal(t, javaScriptPicturePath, cssPicturePath)

	servedPicture := readServeOutput(t, watchServer, javaScriptPicturePath[1:])
	assert.Equal(t, pictureContents, servedPicture)
}

func TestFindEntrypointsUsesEntrypointInsteadOfChunk(t *testing.T) {
	metafile, err := json.Marshal(watchMetafile{
		Outputs: map[string]watchMetafileOutput{
			"chunk-CHUNK.js": {},
			"my-plugin-ENTRY.js": {
				EntryPoint: "Resources/app/administration/src/main.js",
				CSSBundle:  "my-plugin-STYLES.css",
			},
			"my-plugin-STYLES.css": {},
		},
	})
	require.NoError(t, err)

	entrypoints, err := findEntrypoints(api.BuildResult{Metafile: string(metafile)})

	require.NoError(t, err)
	assert.Equal(t, "/my-plugin-ENTRY.js", entrypoints.JavaScript)
	assert.Equal(t, "/my-plugin-STYLES.css", entrypoints.CSS)
}

func TestFindEntrypointsAllowsMissingCSS(t *testing.T) {
	metafile, err := json.Marshal(watchMetafile{
		Outputs: map[string]watchMetafileOutput{
			"my-plugin-ENTRY.js": {
				EntryPoint: "Resources/app/administration/src/main.js",
			},
		},
	})
	require.NoError(t, err)

	entrypoints, err := findEntrypoints(api.BuildResult{Metafile: string(metafile)})

	require.NoError(t, err)
	assert.Equal(t, "/my-plugin-ENTRY.js", entrypoints.JavaScript)
	assert.Empty(t, entrypoints.CSS)
}

func TestRebuildEntrypointsReturnsBuildError(t *testing.T) {
	buildContext := staticBuildContext{
		result: api.BuildResult{
			Errors: []api.Message{{Text: "build failed"}},
		},
	}

	_, err := RebuildEntrypoints(buildContext)

	assert.EqualError(t, err, "build failed")
}

func TestFindEntrypointsReturnsMetadataErrors(t *testing.T) {
	t.Run("malformed metafile", func(t *testing.T) {
		_, err := findEntrypoints(api.BuildResult{Metafile: `{"outputs":`})

		assert.ErrorContains(t, err, "cannot decode esbuild metafile")
	})

	t.Run("missing JavaScript entrypoint", func(t *testing.T) {
		metafile, err := json.Marshal(watchMetafile{
			Outputs: map[string]watchMetafileOutput{
				"my-plugin.css": {},
			},
		})
		require.NoError(t, err)

		_, err = findEntrypoints(api.BuildResult{Metafile: string(metafile)})

		assert.EqualError(t, err, "esbuild emitted no JavaScript entrypoint")
	})

	t.Run("JavaScript outside serve directory", func(t *testing.T) {
		metafile, err := json.Marshal(watchMetafile{
			Outputs: map[string]watchMetafileOutput{
				"../my-plugin.js": {
					EntryPoint: "Resources/app/administration/src/main.js",
				},
			},
		})
		require.NoError(t, err)

		_, err = findEntrypoints(api.BuildResult{Metafile: string(metafile)})

		assert.ErrorContains(t, err, "outside the serve directory")
	})

	t.Run("CSS outside serve directory", func(t *testing.T) {
		metafile, err := json.Marshal(watchMetafile{
			Outputs: map[string]watchMetafileOutput{
				"my-plugin.js": {
					EntryPoint: "Resources/app/administration/src/main.js",
					CSSBundle:  "../my-plugin.css",
				},
			},
		})
		require.NoError(t, err)

		_, err = findEntrypoints(api.BuildResult{Metafile: string(metafile)})

		assert.ErrorContains(t, err, "outside the serve directory")
	})
}

func TestServePath(t *testing.T) {
	t.Run("absolute path in serve directory", func(t *testing.T) {
		absolutePath, err := filepath.Abs(filepath.Join("chunks", "chunk.js"))
		require.NoError(t, err)

		resultPath, err := servePath(absolutePath)

		require.NoError(t, err)
		assert.Equal(t, "/chunks/chunk.js", resultPath)
	})

	t.Run("parent traversal", func(t *testing.T) {
		_, err := servePath(filepath.Join("..", "chunk.js"))

		assert.ErrorContains(t, err, "outside the serve directory")
	})
}

type staticBuildContext struct {
	result api.BuildResult
}

func (c staticBuildContext) Rebuild() api.BuildResult {
	return c.result
}

func (staticBuildContext) Watch(api.WatchOptions) error {
	return nil
}

func (staticBuildContext) Serve(api.ServeOptions) (api.ServeResult, error) {
	return api.ServeResult{}, nil
}

func (staticBuildContext) Cancel() {}

func (staticBuildContext) Dispose() {}

func readServeOutput(t *testing.T, watchServer api.ServeResult, outputPath string) []byte {
	t.Helper()

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		fmt.Sprintf("http://%s:%d%s", watchServer.Hosts[0], watchServer.Port, outputPath),
		nil,
	)
	require.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	require.NoError(t, readErr)
	require.NoError(t, closeErr)
	assert.Equal(t, http.StatusOK, response.StatusCode)

	return body
}
