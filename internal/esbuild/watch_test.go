package esbuild

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuildEntrypointsReturnsContentAddressedPaths(t *testing.T) {
	dir := t.TempDir()
	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	require.NoError(t, os.MkdirAll(adminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("import './style.css'; console.log('served-marker')"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "style.css"), []byte(".served-marker { color: red; }"), 0o644))

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

	for _, test := range []struct {
		name   string
		path   string
		marker string
	}{
		{name: "javascript", path: entrypoints.JavaScript, marker: "served-marker"},
		{name: "css", path: entrypoints.CSS, marker: ".served-marker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				fmt.Sprintf("http://%s:%d%s", watchServer.Hosts[0], watchServer.Port, test.path),
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
			assert.Contains(t, string(body), test.marker)
		})
	}
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
