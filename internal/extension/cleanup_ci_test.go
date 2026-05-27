package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupAdministrationFiles_RemovesSourceFiles(t *testing.T) {
	tmpDir := t.TempDir()
	adminDir := filepath.Join(tmpDir, "Resources", "app", "administration")

	// Create admin source structure
	require.NoError(t, os.MkdirAll(filepath.Join(adminDir, "src", "module", "test"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "src", "main.js"), []byte("import './module/test'"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "src", "module", "test", "index.js"), []byte("export default {}"), 0644))

	err := CleanupAdministrationFiles(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify new empty main.js exists
	mainJsPath := filepath.Join(adminDir, "src", "main.js")
	assert.FileExists(t, mainJsPath)

	content, err := os.ReadFile(mainJsPath)
	require.NoError(t, err)
	assert.Empty(t, content)
}

func TestCleanupAdministrationFiles_MergesSnippetFiles(t *testing.T) {
	tmpDir := t.TempDir()
	adminDir := filepath.Join(tmpDir, "Resources", "app", "administration")

	// Create snippet structure with multiple locale files
	snippetDir1 := filepath.Join(adminDir, "src", "app", "snippet")
	snippetDir2 := filepath.Join(adminDir, "src", "module", "test", "snippet")

	require.NoError(t, os.MkdirAll(snippetDir1, 0755))
	require.NoError(t, os.MkdirAll(snippetDir2, 0755))

	// Create locale snippet files
	snippet1 := map[string]string{"key1": "value1"}
	snippet2 := map[string]string{"key2": "value2"}

	data1, _ := json.Marshal(snippet1)
	data2, _ := json.Marshal(snippet2)

	require.NoError(t, os.WriteFile(filepath.Join(snippetDir1, "en-GB.json"), data1, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(snippetDir2, "en-GB.json"), data2, 0644))

	err := CleanupAdministrationFiles(context.Background(), tmpDir)
	require.NoError(t, err)

	// Verify merged snippet file exists
	mergedSnippetPath := filepath.Join(snippetDir1, "en-GB.json")
	assert.FileExists(t, mergedSnippetPath)

	// Verify merged content contains both keys
	mergedContent, err := os.ReadFile(mergedSnippetPath)
	require.NoError(t, err)

	var merged map[string]string
	require.NoError(t, json.Unmarshal(mergedContent, &merged))

	assert.Equal(t, "value1", merged["key1"])
	assert.Equal(t, "value2", merged["key2"])
}

func TestCleanupAdministrationFiles_NoAdminFolder(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, CleanupAdministrationFiles(context.Background(), tmpDir))
}

func TestCleanupJavaScriptSourceMaps_RemovesMapFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .js and .js.map files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "app.js"), []byte("console.log('test')//# sourceMappingURL=app.js.map"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "app.js.map"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "vendor.js"), []byte("var x = 1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "vendor.js.map"), []byte("{}"), 0644))

	require.NoError(t, CleanupJavaScriptSourceMaps(tmpDir))

	// Verify .js.map files are deleted
	assert.NoFileExists(t, filepath.Join(tmpDir, "app.js.map"))
	assert.NoFileExists(t, filepath.Join(tmpDir, "vendor.js.map"))

	// Verify sourceMappingURL comments are removed from .js files
	appJs, err := os.ReadFile(filepath.Join(tmpDir, "app.js"))
	require.NoError(t, err)
	assert.NotContains(t, string(appJs), "sourceMappingURL")

	vendorJs, err := os.ReadFile(filepath.Join(tmpDir, "vendor.js"))
	require.NoError(t, err)
	assert.Equal(t, "var x = 1", string(vendorJs))
}

func TestCleanupJavaScriptSourceMaps_PreservesNonMapFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "app.js"), []byte("console.log('test')"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "style.css"), []byte("body {}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "image.png"), []byte{}, 0644))

	require.NoError(t, CleanupJavaScriptSourceMaps(tmpDir))

	// All files should still exist
	assert.FileExists(t, filepath.Join(tmpDir, "app.js"))
	assert.FileExists(t, filepath.Join(tmpDir, "style.css"))
	assert.FileExists(t, filepath.Join(tmpDir, "image.png"))
}

func TestCleanupJavaScriptSourceMaps_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	nestedDir := filepath.Join(tmpDir, "assets", "js")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "app.js"), []byte("console.log('test')//# sourceMappingURL=app.js.map"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "app.js.map"), []byte("{}"), 0644))

	require.NoError(t, CleanupJavaScriptSourceMaps(tmpDir))

	assert.NoFileExists(t, filepath.Join(nestedDir, "app.js.map"))
}

func TestCleanupJavaScriptSourceMaps_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, CleanupJavaScriptSourceMaps(tmpDir))
}

func TestCleanupJavaScriptSourceMaps_NonExistentDirectory(t *testing.T) {
	require.NoError(t, CleanupJavaScriptSourceMaps("/non/existent/path"))
}
