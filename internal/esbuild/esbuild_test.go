package esbuild

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/logging"
)

func getTestContext() context.Context {
	logger := logging.NewLogger(false)

	return logging.WithLogger(context.TODO(), logger)
}

func TestESBuildAdminNoEntrypoint(t *testing.T) {
	dir := t.TempDir()

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	_, err := CompileExtensionAsset(getTestContext(), options)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find entrypoint")
}

func TestESBuildAdmin(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	_ = os.MkdirAll(adminDir, 0o755)

	_ = os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("console.log('bla')"), 0o644)

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	options.DisableSass = true
	result, err := CompileExtensionAsset(getTestContext(), options)

	assert.NoError(t, err)

	compiledFilePath := filepath.Join(dir, "Resources", "public", "administration", "js", "bla.js")
	_, err = os.Stat(compiledFilePath)
	assert.NoError(t, err)

	assert.NotEmpty(t, result.HashedJsFile)
	hashedFilePath := filepath.Join(dir, "Resources", "public", "administration", result.HashedJsFile)
	_, err = os.Stat(hashedFilePath)
	assert.NoError(t, err)
}

func TestESBuildAdminWithSCSS(t *testing.T) {
	if !IsDartSassAvailable() {
		t.Skip("dart-sass not available locally; install it or run once with network to cache")
	}

	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	_ = os.MkdirAll(adminDir, 0o755)

	_ = os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("import './a.scss'"), 0o644)
	_ = os.WriteFile(filepath.Join(adminDir, "a.scss"), []byte(".a { .b { color: red } }"), 0o644)

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	result, err := CompileExtensionAsset(getTestContext(), options)

	assert.NoError(t, err)

	compiledFilePathJS := filepath.Join(dir, "Resources", "public", "administration", "js", "bla.js")
	_, err = os.Stat(compiledFilePathJS)
	assert.NoError(t, err)

	compiledFilePathCSS := filepath.Join(dir, "Resources", "public", "administration", "css", "bla.css")
	_, err = os.Stat(compiledFilePathCSS)
	assert.NoError(t, err)

	assert.NotEmpty(t, result.HashedJsFile)
	assert.NotEmpty(t, result.HashedCssFile)

	hashedJSPath := filepath.Join(dir, "Resources", "public", "administration", result.HashedJsFile)
	_, err = os.Stat(hashedJSPath)
	assert.NoError(t, err)

	hashedCSSPath := filepath.Join(dir, "Resources", "public", "administration", result.HashedCssFile)
	_, err = os.Stat(hashedCSSPath)
	assert.NoError(t, err)

	bytes, err := os.ReadFile(compiledFilePathCSS)
	assert.NoError(t, err)

	assert.Contains(t, string(bytes), ".a .b")
}

func TestESBuildAdminTypeScript(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	_ = os.MkdirAll(adminDir, 0o755)

	_ = os.WriteFile(filepath.Join(adminDir, "main.ts"), []byte("console.log('bla')"), 0o644)

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	options.DisableSass = true
	result, err := CompileExtensionAsset(getTestContext(), options)

	assert.NoError(t, err)
	assert.Contains(t, result.Entrypoint, "main.ts")

	compiledFilePath := filepath.Join(dir, "Resources", "public", "administration", "js", "bla.js")
	_, err = os.Stat(compiledFilePath)
	assert.NoError(t, err)

	assert.NotEmpty(t, result.HashedJsFile)
	hashedFilePath := filepath.Join(dir, "Resources", "public", "administration", result.HashedJsFile)
	_, err = os.Stat(hashedFilePath)
	assert.NoError(t, err)
}

func TestESBuildStorefront(t *testing.T) {
	dir := t.TempDir()

	storefrontDir := filepath.Join(dir, "Resources", "app", "storefront", "src")
	_ = os.MkdirAll(storefrontDir, 0o755)

	_ = os.WriteFile(filepath.Join(storefrontDir, "main.js"), []byte("console.log('bla')"), 0o644)

	options := NewAssetCompileOptionsStorefront("Bla", dir, false)
	result, err := CompileExtensionAsset(getTestContext(), options)

	assert.NoError(t, err)

	compiledFilePath := filepath.Join(dir, "Resources", "app", "storefront", "dist", "storefront", "js", "bla.js")
	_, err = os.Stat(compiledFilePath)
	assert.NoError(t, err)

	assert.NotEmpty(t, result.HashedJsFile)
	hashedFilePath := filepath.Join(dir, "Resources", "app", "storefront", "dist", "storefront", result.HashedJsFile)
	_, err = os.Stat(hashedFilePath)
	assert.NoError(t, err)
}

func TestESBuildStorefrontNewLayout(t *testing.T) {
	dir := t.TempDir()

	storefrontDir := filepath.Join(dir, "Resources", "app", "storefront", "src")
	_ = os.MkdirAll(storefrontDir, 0o755)

	_ = os.WriteFile(filepath.Join(storefrontDir, "main.js"), []byte("console.log('bla')"), 0o644)

	options := NewAssetCompileOptionsStorefront("Bla", dir, true)
	result, err := CompileExtensionAsset(getTestContext(), options)

	assert.NoError(t, err)

	compiledFilePath := filepath.Join(dir, "Resources", "app", "storefront", "dist", "storefront", "js", "bla", "bla.js")
	_, err = os.Stat(compiledFilePath)
	assert.NoError(t, err)

	assert.NotEmpty(t, result.HashedJsFile)
	hashedFilePath := filepath.Join(dir, "Resources", "app", "storefront", "dist", "storefront", result.HashedJsFile)
	_, err = os.Stat(hashedFilePath)
	assert.NoError(t, err)
}
