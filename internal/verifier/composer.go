package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string) error {
	suggets := getComposerSuggets(rootDir)

	if _, err := os.Stat(path.Join(rootDir, "vendor")); os.IsNotExist(err) {
		composerAuth, err := packagist.ReadComposerAuth(path.Join(rootDir, "auth.json"))
		if err != nil {
			return fmt.Errorf("failed to read composer auth file: %w", err)
		}

		encoded, err := composerAuth.Json(false)
		if err != nil {
			return fmt.Errorf("failed to encode composer auth: %w", err)
		}

		if len(suggets) > 0 {
			additionalParams := []string{"require", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}
			for _, suggest := range suggets {
				additionalParams = append(additionalParams, fmt.Sprintf("%s:*", suggest))
			}

			composerInstall := exec.CommandContext(ctx, "composer", additionalParams...)
			composerInstall.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
			composerInstall.Dir = rootDir

			log, err := composerInstall.CombinedOutput()
			if err != nil {
				if _, writeErr := os.Stderr.Write(log); writeErr != nil {
					return fmt.Errorf("failed to write error log: %w (original error: %v)", writeErr, err)
				}
				return err
			}
		}

		additionalParams := []string{"update", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}

		if checkAgainst == "lowest" {
			additionalParams = append(additionalParams, "--prefer-lowest")
		}

		composerInstall := exec.CommandContext(ctx, "composer", additionalParams...)
		composerInstall.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
		composerInstall.Dir = rootDir

		log, err := composerInstall.CombinedOutput()
		if err != nil {
			if _, writeErr := os.Stderr.Write(log); writeErr != nil {
				return fmt.Errorf("failed to write error log: %w (original error: %v)", writeErr, err)
			}
			return err
		}
	}

	return nil
}

func getComposerSuggets(rootDir string) []string {
	composerJSON, err := os.ReadFile(path.Join(rootDir, "composer.json"))
	if err != nil {
		return []string{}
	}

	var composerJSONData map[string]interface{}
	if err := json.Unmarshal(composerJSON, &composerJSONData); err != nil {
		return []string{}
	}

	if composerJSONData["suggest"] == nil {
		return []string{}
	}

	suggests := make([]string, 0, len(composerJSONData["suggest"].(map[string]interface{})))
	for k := range composerJSONData["suggest"].(map[string]interface{}) {
		suggests = append(suggests, k)
	}

	return suggests
}
