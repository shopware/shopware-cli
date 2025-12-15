package esbuild

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

type ViteManifestFile struct {
	File    string   `json:"file"`
	Name    string   `json:"name"`
	Src     string   `json:"src"`
	IsEntry bool     `json:"isEntry"`
	Css     []string `json:"css"`
}

type ViteManifest struct {
	MainJs ViteManifestFile `json:"main.js"`
}

func dumpViteManifest(options AssetCompileOptions, viteDir string) error {
	m := ViteManifest{
		MainJs: ViteManifestFile{
			File:    options.OutputJSFile,
			Name:    ToKebabCase(options.Name),
			Src:     "main.js",
			IsEntry: true,
			Css:     []string{options.OutputCSSFile},
		},
	}

	j, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path.Join(viteDir, "manifest.json"), j, os.ModePerm)
}

type ViteEntrypoint struct {
	Css     []string `json:"css"`
	Dynamic []string `json:"dynamic"`
	Js      []string `json:"js"`
	Legacy  bool     `json:"legacy"`
	Preload []string `json:"preload"`
}

type ViteEntrypoints struct {
	Base        string                    `json:"base"`
	EntryPoints map[string]ViteEntrypoint `json:"entryPoints"`
	Legacy      bool                      `json:"legacy"`
	Metadata    interface{}               `json:"metadatas"`
	Version     []interface{}             `json:"version"`
	ViteServer  *interface{}              `json:"viteServer"`
}

func dumpViteEntrypoint(options AssetCompileOptions, viteDir string) error {
	bundleFolderName := toBundleFolderName(options.Name)

	e := ViteEntrypoints{
		Base: fmt.Sprintf("/bundles/%s/administration/", bundleFolderName),
		EntryPoints: map[string]ViteEntrypoint{
			ToKebabCase(options.Name): {
				Css: []string{
					fmt.Sprintf("/bundles/%s/administration/%s", bundleFolderName, options.OutputCSSFile),
				},
				Dynamic: []string{},
				Js: []string{
					fmt.Sprintf("/bundles/%s/administration/%s", bundleFolderName, options.OutputJSFile),
				},
				Legacy:  false,
				Preload: []string{},
			},
		},
		Legacy:   false,
		Metadata: map[string]interface{}{},
		Version: []interface{}{
			"7.0.4",
			7,
			0,
			4,
		},
		ViteServer: nil,
	}

	j, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path.Join(viteDir, "entrypoints.json"), j, os.ModePerm)
}

func DumpViteConfig(options AssetCompileOptions) error {
	viteDir := path.Join(path.Join(options.Path, options.OutputDir), ".vite")

	if _, err := os.Stat(viteDir); os.IsNotExist(err) {
		if err := os.MkdirAll(viteDir, 0o755); err != nil {
			return fmt.Errorf("failed to create Vite directory: %w", err)
		}
	}

	// Instead of throwing an error, just skip overwriting if the config exists
	manifestPath := path.Join(viteDir, "manifest.json")
	entrypointsPath := path.Join(viteDir, "entrypoints.json")
	if _, err := os.Stat(manifestPath); err == nil {
		return nil
	}
	if _, err := os.Stat(entrypointsPath); err == nil {
		return nil
	}

	if err := dumpViteManifest(options, viteDir); err != nil {
		return err
	}

	return dumpViteEntrypoint(options, viteDir)
}
