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

func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string, result *ToolResult) error {
	suggets := getComposerSuggets(rootDir)

	if _, err := os.Stat(path.Join(rootDir, "vendor")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot read vendor directory: %w", err)
		}
		composerAuth, err := shop.ReadComposerAuth(path.Join(rootDir, "auth.json"))
		if err != nil {
			return fmt.Errorf("cannot read Composer auth file: %w", err)
		}

		encoded, err := composerAuth.Json(false)
		if err != nil {
			return fmt.Errorf("cannot encode Composer auth: %w", err)
		}

		if len(suggets) > 0 {
			additionalParams := []string{"require", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}
			for _, suggest := range suggets {
				additionalParams = append(additionalParams, suggest+":*")
			}

			composerInstall := CommandContext(ctx, "composer", additionalParams...)
			composerInstall.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
			composerInstall.Dir = rootDir

			_, _, err = captureCommand(composerInstall, result)
			if err != nil {
				return fmt.Errorf("cannot install Composer suggestions: %w", err)
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

		_, _, err = captureCommand(composerInstall, result)
		if err != nil {
			return fmt.Errorf("cannot install Composer dependencies: %w", err)
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
