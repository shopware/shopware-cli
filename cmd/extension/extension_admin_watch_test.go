package extension

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/esbuild"
	"github.com/shopware/shopware-cli/logging"
)

func TestServeCurrentOutputServesHashedBundleUnderStableName(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	assert.NoError(t, os.MkdirAll(adminDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("console.log('served-marker')"), 0o644))

	ctx := logging.WithLogger(t.Context(), logging.NewLogger(false))

	options := esbuild.NewAssetCompileOptionsAdmin("MyPlugin", dir)
	options.ProductionMode = false
	options.DisableSass = true

	esbuildContext, err := esbuild.Context(ctx, options)
	assert.Nil(t, err)
	defer esbuildContext.Dispose()

	ext := adminWatchExtension{
		name:      "MyPlugin",
		assetName: "my-plugin",
		context:   esbuildContext,
	}

	// esbuild emits a content-hashed filename, but the watcher must serve it under the
	// stable "extension.js" URL that the administration requests.
	rec := httptest.NewRecorder()
	ext.serveCurrentOutput(ctx, rec, ".js", "application/javascript")

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/javascript", rec.Header().Get("content-type"))
	assert.Contains(t, rec.Body.String(), "served-marker")
}

func TestServeCurrentOutputReturns404WhenBundleMissing(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	assert.NoError(t, os.MkdirAll(adminDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("console.log('served-marker')"), 0o644))

	ctx := logging.WithLogger(t.Context(), logging.NewLogger(false))

	options := esbuild.NewAssetCompileOptionsAdmin("MyPlugin", dir)
	options.ProductionMode = false
	options.DisableSass = true

	esbuildContext, err := esbuild.Context(ctx, options)
	assert.Nil(t, err)
	defer esbuildContext.Dispose()

	ext := adminWatchExtension{
		name:      "MyPlugin",
		assetName: "my-plugin",
		context:   esbuildContext,
	}

	// No CSS is imported, so there is no CSS output to serve.
	rec := httptest.NewRecorder()
	ext.serveCurrentOutput(ctx, rec, ".css", "text/css")

	assert.Equal(t, 404, rec.Code)
}
