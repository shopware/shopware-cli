package esbuild

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDumpViteManifest(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Define sample AssetCompileOptions
	options := AssetCompileOptions{
		OutputJSFile:  "main.js",
		OutputCSSFile: "styles.css",
		Name:          "TestName",
		Path:          tempDir,
		OutputDir:     "dist",
	}

	// Call dumpViteManifest
	err := dumpViteManifest(options, tempDir)
	assert.NoError(t, err)

	// Verify that manifest.json is created
	manifestPath := path.Join(tempDir, "manifest.json")
	_, err = os.Stat(manifestPath)
	assert.NoError(t, err)

	// Read and unmarshal the content of manifest.json
	content, err := os.ReadFile(manifestPath)
	assert.NoError(t, err)

	var manifest ViteManifest
	err = json.Unmarshal(content, &manifest)
	assert.NoError(t, err)

	// Assert the content of manifest.json
	expectedManifest := ViteManifest{
		MainJs: ViteManifestFile{
			File:    "main.js",
			Name:    "test-name",
			Src:     "main.js",
			IsEntry: true,
			Css:     []string{"styles.css"},
		},
	}
	assert.Equal(t, expectedManifest, manifest)
}

func TestDumpViteEntrypoint(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Define sample AssetCompileOptions
	options := AssetCompileOptions{
		OutputJSFile:  "main.js",
		OutputCSSFile: "styles.css",
		Name:          "TestName",
		Path:          tempDir,
		OutputDir:     "dist",
	}

	// Call dumpViteEntrypoint
	err := dumpViteEntrypoint(options, tempDir)
	assert.NoError(t, err)

	// Verify that entrypoints.json is created
	entrypointsPath := path.Join(tempDir, "entrypoints.json")
	_, err = os.Stat(entrypointsPath)
	assert.NoError(t, err)

	// Read and unmarshal the content of entrypoints.json
	content, err := os.ReadFile(entrypointsPath)
	assert.NoError(t, err)

	var entrypoints ViteEntrypoints
	err = json.Unmarshal(content, &entrypoints)
	assert.NoError(t, err)

	// Assert the content of entrypoints.json
	expectedEntrypoints := ViteEntrypoints{
		Base: "/bundles/testname/administration/",
		EntryPoints: map[string]ViteEntrypoint{
			"test-name": {
				Css:     []string{"/bundles/testname/administration/styles.css"},
				Dynamic: []string{},
				Js:      []string{"/bundles/testname/administration/main.js"},
				Legacy:  false,
				Preload: []string{},
			},
		},
		Legacy:   false,
		Metadata: map[string]interface{}{},
		Version: []interface{}{
			"7.0.4",
			float64(7),
			float64(0),
			float64(4),
		},
		ViteServer: nil,
	}

	assert.Equal(t, expectedEntrypoints, entrypoints)
}

func TestDumpViteConfig(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Define sample AssetCompileOptions
	options := AssetCompileOptions{
		OutputJSFile:  "main.js",
		OutputCSSFile: "styles.css",
		Name:          "TestName",
		Path:          tempDir,
		OutputDir:     "dist",
	}

	// Call DumpViteConfig
	err := DumpViteConfig(options)
	assert.NoError(t, err)

	// Verify that .vite directory is created
	viteDir := path.Join(tempDir, "dist", ".vite")
	stat, err := os.Stat(viteDir)
	assert.NoError(t, err)
	assert.True(t, stat.IsDir())

	// Verify that both manifest.json and entrypoints.json exist
	_, err = os.Stat(path.Join(viteDir, "manifest.json"))
	assert.NoError(t, err)

	_, err = os.Stat(path.Join(viteDir, "entrypoints.json"))
	assert.NoError(t, err)
}
