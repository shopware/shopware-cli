package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// TargetCopySkips stay out of the temporary copy so Composer installs the target release.
var TargetCopySkips = []string{"vendor", "composer.lock"}

// shopwarePlatformPackages share one version number with shopware/core.
var shopwarePlatformPackages = []string{"shopware/core", "shopware/administration", "shopware/storefront", "shopware/elasticsearch"}

// installComposerDeps installs vendor/ when missing; a non-empty pinVersion pins the Shopware packages first.
func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string, pinVersion string) error {
	suggets := getComposerSuggets(rootDir)

	if _, err := os.Stat(path.Join(rootDir, "vendor")); os.IsNotExist(err) {
		composerAuth, err := shop.ReadComposerAuth(path.Join(rootDir, "auth.json"))
		if err != nil {
			return fmt.Errorf("failed to read composer auth file: %w", err)
		}

		encoded, err := composerAuth.Json(false)
		if err != nil {
			return fmt.Errorf("failed to encode composer auth: %w", err)
		}

		if pins := shopwarePlatformPins(rootDir, pinVersion); len(pins) > 0 {
			composerPin := exec.CommandContext(ctx, "composer", append([]string{"require", "--no-update", "--no-interaction", "--no-plugins", "--no-scripts"}, pins...)...)
			composerPin.Env = append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
			composerPin.Dir = rootDir

			log, err := composerPin.CombinedOutput()
			if err != nil {
				if _, writeErr := os.Stderr.Write(log); writeErr != nil {
					return fmt.Errorf("failed to write error log: %w (original error: %v)", writeErr, err)
				}
				return fmt.Errorf("failed to pin the Shopware packages to %s: %w", pinVersion, err)
			}
		}

		if len(suggets) > 0 {
			additionalParams := []string{"require", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}
			for _, suggest := range suggets {
				additionalParams = append(additionalParams, suggest+":*")
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

		beforeStart := time.Now()

		log, err := composerInstall.CombinedOutput()
		if err != nil {
			if _, writeErr := os.Stderr.Write(log); writeErr != nil {
				return fmt.Errorf("failed to write error log: %w (original error: %v)", writeErr, err)
			}
			return err
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

// shopwarePlatformPins lists the required Shopware platform packages pinned to the version.
func shopwarePlatformPins(rootDir string, pinVersion string) []string {
	if pinVersion == "" {
		return nil
	}

	composerJSON, err := os.ReadFile(path.Join(rootDir, "composer.json"))
	if err != nil {
		return nil
	}

	var composerJSONData struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(composerJSON, &composerJSONData); err != nil {
		return nil
	}

	pins := make([]string, 0, len(shopwarePlatformPackages))
	for _, pkg := range shopwarePlatformPackages {
		if _, ok := composerJSONData.Require[pkg]; ok {
			pins = append(pins, pkg+":"+pinVersion)
		}
	}

	return pins
}

// installedShopwareVersion reads the shopware/core version from vendor/composer/installed.json.
func installedShopwareVersion(rootDir string) string {
	data, err := os.ReadFile(path.Join(rootDir, "vendor", "composer", "installed.json"))
	if err != nil {
		return ""
	}

	type installedPackage struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	var installed struct {
		Packages []installedPackage `json:"packages"`
	}

	// Composer 1 wrote a bare array, Composer 2 wraps it in an object
	if err := json.Unmarshal(data, &installed); err != nil {
		if err := json.Unmarshal(data, &installed.Packages); err != nil {
			return ""
		}
	}

	for _, pkg := range installed.Packages {
		if pkg.Name == "shopware/core" {
			return strings.TrimPrefix(pkg.Version, "v")
		}
	}

	return ""
}
