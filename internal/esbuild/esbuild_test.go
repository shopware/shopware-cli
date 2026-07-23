package esbuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestESBuildAdminWithCSS(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	_ = os.MkdirAll(adminDir, 0o755)

	_ = os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("import './a.css'"), 0o644)
	_ = os.WriteFile(filepath.Join(adminDir, "a.css"), []byte(".a { color: red; }"), 0o644)

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	options.DisableSass = true
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
}

func TestESBuildAdminWithFileAsset(t *testing.T) {
	dir := t.TempDir()

	adminDir := filepath.Join(dir, "Resources", "app", "administration", "src")
	require.NoError(t, os.MkdirAll(adminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("import picture from './picture.png'; import './style.css'; console.log(picture)"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "style.css"), []byte(".picture { background-image: url('./picture.png'); }"), 0o644))
	pictureContents := []byte("picture-contents")
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "picture.png"), pictureContents, 0o644))

	options := NewAssetCompileOptionsAdmin("Bla", dir)
	options.DisableSass = true
	result, err := CompileExtensionAsset(getTestContext(), options)
	require.NoError(t, err)

	assert.Regexp(t, `^js/bla-[A-Z0-9]+\.js$`, filepath.ToSlash(result.HashedJsFile))
	assert.Regexp(t, `^css/bla-[A-Z0-9]+\.css$`, filepath.ToSlash(result.HashedCssFile))

	outputDir := filepath.Join(dir, "Resources", "public", "administration")
	compiledJS, err := os.ReadFile(filepath.Join(outputDir, "js", "bla.js"))
	require.NoError(t, err)
	compiledCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "bla.css"))
	require.NoError(t, err)

	picturePathPattern := regexp.MustCompile(`\./picture-[A-Z0-9]+\.png`)
	javaScriptPicturePath := picturePathPattern.FindString(string(compiledJS))
	cssPicturePath := picturePathPattern.FindString(string(compiledCSS))
	require.NotEmpty(t, javaScriptPicturePath)
	assert.Equal(t, javaScriptPicturePath, cssPicturePath)

	for _, outputFolder := range []string{"js", "css"} {
		writtenPicture, err := os.ReadFile(filepath.Join(outputDir, outputFolder, javaScriptPicturePath[2:]))
		require.NoError(t, err)
		assert.Equal(t, pictureContents, writtenPicture)
	}
}

func TestWriteBundlerResultToDiskDoesNotTreatAssetAsEntrypoint(t *testing.T) {
	entryPath, err := filepath.Abs("bla-ENTRY.js")
	require.NoError(t, err)
	picturePath, err := filepath.Abs("picture-ASSET.png")
	require.NoError(t, err)

	metafile, err := json.Marshal(watchMetafile{
		Outputs: map[string]watchMetafileOutput{
			"bla-ENTRY.js": {
				EntryPoint: "Resources/app/administration/src/main.js",
			},
			"picture-ASSET.png": {},
		},
	})
	require.NoError(t, err)

	options := AssetCompileOptions{
		Path:          t.TempDir(),
		OutputDir:     "dist",
		OutputJSFile:  "js/bla.js",
		OutputCSSFile: "css/bla.css",
	}
	hashedJS, hashedCSS, err := writeBundlerResultToDisk(api.BuildResult{
		Metafile: string(metafile),
		OutputFiles: []api.OutputFile{
			{Path: entryPath, Contents: []byte("entry")},
			{Path: picturePath, Contents: []byte("picture")},
		},
	}, options)
	require.NoError(t, err)

	assert.Equal(t, filepath.FromSlash("js/bla-ENTRY.js"), hashedJS)
	assert.Empty(t, hashedCSS)

	stableEntry, err := os.ReadFile(filepath.Join(options.Path, options.OutputDir, options.OutputJSFile))
	require.NoError(t, err)
	assert.Equal(t, "entry", string(stableEntry))

	writtenPicture, err := os.ReadFile(filepath.Join(options.Path, options.OutputDir, "js", "picture-ASSET.png"))
	require.NoError(t, err)
	assert.Equal(t, "picture", string(writtenPicture))
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
