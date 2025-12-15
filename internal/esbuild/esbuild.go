package esbuild

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

type AssetCompileResult struct {
	Name       string
	Entrypoint string
	JsFile     string
	CssFile    string
}

type AssetCompileOptions struct {
	ProductionMode  bool
	DisableSass     bool
	EntrypointDir   string
	OutputDir       string
	Name            string
	Path            string
	OutputJSFile    string
	OutputCSSFile   string
	StaticSourceDir string
	StaticTargetDir string
}

const DotJs = ".js"

func NewAssetCompileOptionsAdmin(name, path string) AssetCompileOptions {
	kebabCased := ToKebabCase(name)

	return AssetCompileOptions{
		Name:            name,
		Path:            path,
		EntrypointDir:   "Resources/app/administration/src",
		StaticSourceDir: "Resources/app/administration/static",
		StaticTargetDir: "Resources/public/static",
		OutputDir:       "Resources/public/administration",
		ProductionMode:  true,
		OutputJSFile:    filepath.Join("js", kebabCased+DotJs),
		OutputCSSFile:   filepath.Join("css", kebabCased+".css"),
	}
}

func NewAssetCompileOptionsStorefront(name, path string, newLayout bool) AssetCompileOptions {
	kebabCased := ToKebabCase(name)

	jsFile := filepath.Join("js", kebabCased+DotJs)

	if newLayout {
		jsFile = filepath.Join("js", kebabCased, kebabCased+DotJs)
	}

	return AssetCompileOptions{
		Name:           name,
		Path:           path,
		EntrypointDir:  "Resources/app/storefront/src",
		OutputDir:      "Resources/app/storefront/dist/storefront",
		ProductionMode: true,
		OutputJSFile:   jsFile,
		OutputCSSFile:  filepath.Join("css", kebabCased+".css"),
		// We never emit CSS for the storefront, they are always lying in a separate SCSS file entrypoint
		DisableSass: true,
	}
}

func getEsbuildOptions(ctx context.Context, options AssetCompileOptions) (*api.BuildOptions, error) {
	entryPoint := filepath.Join(options.Path, options.EntrypointDir, "main.js")

	if _, err := os.Stat(entryPoint); os.IsNotExist(err) {
		entryPointTS := filepath.Join(options.Path, options.EntrypointDir, "main.ts")

		if _, err := os.Stat(entryPointTS); os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot find entrypoint at %s as main.js or main.ts", options.EntrypointDir)
		}

		entryPoint = entryPointTS
	}

	plugins := []api.Plugin{}
	loader := map[string]api.Loader{
		".html": api.LoaderText,
		".twig": api.LoaderText,
		".css":  api.LoaderCSS,
		".png":  api.LoaderFile,
		".jpg":  api.LoaderFile,
		".jpeg": api.LoaderFile,
		".svg":  api.LoaderFile,
		".ts":   api.LoaderTS,
	}

	if !options.DisableSass {
		plugins = append(plugins, newScssPlugin(ctx))
		loader[".scss"] = api.LoaderCSS
	}

	bundlerOptions := api.BuildOptions{
		MinifySyntax:      options.ProductionMode,
		MinifyWhitespace:  options.ProductionMode,
		MinifyIdentifiers: options.ProductionMode,
		EntryPoints:       []string{entryPoint},
		Outfile:           "extension.js",
		Bundle:            true,
		Write:             false,
		LogLevel:          api.LogLevelWarning,
		Plugins:           plugins,
		Loader:            loader,
	}

	return &bundlerOptions, nil
}

func Context(ctx context.Context, options AssetCompileOptions) (api.BuildContext, *api.ContextError) {
	bundlerOptions, err := getEsbuildOptions(ctx, options)
	if err != nil {
		panic(err)
	}

	return api.Context(*bundlerOptions)
}

func CompileExtensionAsset(ctx context.Context, options AssetCompileOptions) (*AssetCompileResult, error) {
	jsFile := filepath.Join(options.Path, options.OutputDir, options.OutputJSFile)
	cssFile := filepath.Join(options.Path, options.OutputDir, options.OutputCSSFile)

	bundlerOptions, err := getEsbuildOptions(ctx, options)
	if err != nil {
		return nil, err
	}

	result := api.Build(*bundlerOptions)

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("initial compile failed")
	}

	if err := cleanupOutputFolder(options); err != nil {
		return nil, err
	}

	if err := writeBundlerResultToDisk(result, jsFile, cssFile); err != nil {
		return nil, err
	}

	if options.StaticSourceDir != "" && options.StaticTargetDir != "" {
		if err := copyStaticFiles(path.Join(options.Path, options.StaticSourceDir), path.Join(options.Path, options.StaticTargetDir)); err != nil {
			return nil, err
		}
	}

	compileResult := AssetCompileResult{
		Name:       options.Name,
		Entrypoint: bundlerOptions.EntryPoints[0],
		JsFile:     jsFile,
		CssFile:    cssFile,
	}

	return &compileResult, nil
}

func cleanupOutputFolder(options AssetCompileOptions) error {
	folders := []string{"css", "js"}

	for _, folder := range folders {
		folderPath := filepath.Join(options.Path, options.OutputDir, folder)
		if _, err := os.Stat(folderPath); err == nil {
			if err := os.RemoveAll(folderPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeBundlerResultToDisk(result api.BuildResult, jsFile, cssFile string) error {
	for _, file := range result.OutputFiles {
		outFile := jsFile

		if strings.HasSuffix(file.Path, ".css") {
			outFile = cssFile
		}

		outFolder := filepath.Dir(outFile)

		if _, err := os.Stat(outFolder); os.IsNotExist(err) {
			if err := os.MkdirAll(outFolder, os.ModePerm); err != nil {
				return err
			}
		}

		if err := os.WriteFile(outFile, file.Contents, os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}
