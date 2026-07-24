package esbuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

type Entrypoints struct {
	JavaScript string
	CSS        string
}

type watchMetafile struct {
	Outputs map[string]watchMetafileOutput `json:"outputs"`
}

type watchMetafileOutput struct {
	EntryPoint string `json:"entryPoint"`
	CSSBundle  string `json:"cssBundle"`
}

func RebuildEntrypoints(buildContext api.BuildContext) (Entrypoints, error) {
	result := buildContext.Rebuild()

	if len(result.Errors) > 0 {
		return Entrypoints{}, errors.New(result.Errors[0].Text)
	}

	return findEntrypoints(result)
}

func findEntrypoints(result api.BuildResult) (Entrypoints, error) {
	var metafile watchMetafile
	if err := json.Unmarshal([]byte(result.Metafile), &metafile); err != nil {
		return Entrypoints{}, fmt.Errorf("cannot decode esbuild metafile: %w", err)
	}

	for candidatePath, candidate := range metafile.Outputs {
		if candidate.EntryPoint == "" || path.Ext(candidatePath) != ".js" {
			continue
		}

		javaScriptPath, err := servePath(candidatePath)
		if err != nil {
			return Entrypoints{}, fmt.Errorf("cannot resolve JavaScript entrypoint %q: %w", candidatePath, err)
		}

		cssPath := ""
		if candidate.CSSBundle != "" {
			cssPath, err = servePath(candidate.CSSBundle)
			if err != nil {
				return Entrypoints{}, fmt.Errorf("cannot resolve CSS bundle %q: %w", candidate.CSSBundle, err)
			}
		}

		return Entrypoints{
			JavaScript: javaScriptPath,
			CSS:        cssPath,
		}, nil
	}

	return Entrypoints{}, fmt.Errorf("esbuild emitted no JavaScript entrypoint")
}

func servePath(outputPath string) (string, error) {
	cleanOutputPath := filepath.Clean(filepath.FromSlash(outputPath))

	if filepath.IsAbs(cleanOutputPath) {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot determine esbuild serve directory: %w", err)
		}

		relativePath, err := filepath.Rel(workingDirectory, cleanOutputPath)
		if err != nil {
			return "", fmt.Errorf("cannot resolve esbuild output path %q: %w", outputPath, err)
		}
		cleanOutputPath = relativePath
	}

	if cleanOutputPath == ".." || strings.HasPrefix(cleanOutputPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("esbuild output path %q is outside the serve directory", outputPath)
	}

	return "/" + strings.TrimPrefix(filepath.ToSlash(cleanOutputPath), "/"), nil
}
