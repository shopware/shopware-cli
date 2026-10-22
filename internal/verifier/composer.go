package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// preparePHPTool checks whether a tool has PHP sources and a Composer manifest.
func preparePHPTool(config ToolConfig, execution *ToolExecution) (ToolResult, bool) {
	if len(config.SourceDirectories) == 0 {
		return ToolResult{SkipReason: "no source directories"}, false
	}
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ToolResult{SkipReason: "no composer.json found"}, false
		}
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot read composer manifest: %w", err)})
		return ToolResult{}, false
	}
	return ToolResult{}, true
}

func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string, execution *ToolExecution) *ToolError {
	suggets := getComposerSuggets(rootDir)

	if _, err := os.Stat(path.Join(rootDir, "vendor")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return &ToolError{Err: fmt.Errorf("cannot read vendor directory: %w", err)}
		}
		composerAuth, err := shop.ReadComposerAuth(path.Join(rootDir, "auth.json"))
		if err != nil {
			return &ToolError{Err: fmt.Errorf("cannot read Composer auth file: %w", err)}
		}

		encoded, err := composerAuth.Json(false)
		if err != nil {
			return &ToolError{Err: fmt.Errorf("cannot encode Composer auth: %w", err)}
		}

		if len(suggets) > 0 {
			additionalParams := []string{"require", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}
			for _, suggest := range suggets {
				additionalParams = append(additionalParams, suggest+":*")
			}

			composerInstall := CommandContext(ctx, "composer", additionalParams...)
			composerInstall.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
			composerInstall.Dir = rootDir

			subprocess := execution.Run(composerInstall)
			if subprocess.Err != nil {
				return subprocess.ToolError(fmt.Errorf("cannot install Composer suggestions: %w", subprocess.Err))
			}
		}

		additionalParams := []string{"update", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}

		if checkAgainst == "lowest" {
			additionalParams = append(additionalParams, "--prefer-lowest")
		}

		composerInstall := CommandContext(ctx, "composer", additionalParams...)
		composerInstall.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
		composerInstall.Dir = rootDir

		beforeStart := time.Now()

		subprocess := execution.Run(composerInstall)
		if subprocess.Err != nil {
			return subprocess.ToolError(fmt.Errorf("cannot install Composer dependencies: %w", subprocess.Err))
		}

		logging.FromContext(ctx).Debugf("Composer dependencies installed in %s", time.Since(beforeStart).String())
	}

	return nil
}

func getComposerSuggets(rootDir string) []string {
	composerJSON, err := os.ReadFile(path.Join(rootDir, "composer.json"))
	if err != nil {
		return []string{}
	}

	var composerJSONData struct {
		Suggest map[string]string `json:"suggest"`
	}
	if err := json.Unmarshal(composerJSON, &composerJSONData); err != nil {
		return []string{}
	}

	return slices.Sorted(maps.Keys(composerJSONData.Suggest))
}
