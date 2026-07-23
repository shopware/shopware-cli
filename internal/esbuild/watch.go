package esbuild

import (
	"encoding/json"
	"errors"
	"fmt"
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
			return Entrypoints{}, err
		}

		cssPath := ""
		if candidate.CSSBundle != "" {
			cssPath, err = servePath(candidate.CSSBundle)
			if err != nil {
				return Entrypoints{}, err
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
		relativePath, err := filepath.Rel(".", cleanOutputPath)
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
