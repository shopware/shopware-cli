package verifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

// validationVendorTTL keeps "highest" meaningful: after a day the cached
// dependencies are resolved again to pick up new releases.
const validationVendorTTL = 24 * time.Hour

// installComposerDeps installs the dependencies into rootDir itself when vendor/ is missing.
func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string) error {
	if _, err := os.Stat(path.Join(rootDir, "vendor")); !os.IsNotExist(err) {
		return nil
	}

	return runComposerDeps(ctx, rootDir, path.Join(rootDir, "auth.json"), getComposerSuggets(rootDir), checkAgainst)
}

// prepareComposerAutoload returns the autoload file to analyse rootDir with.
// When rootDir has no vendor/, the dependencies are installed into a shared
// cache directory instead, so validation never writes into the user's folder.
func prepareComposerAutoload(ctx context.Context, rootDir string, checkAgainst string) (string, error) {
	if _, err := os.Stat(path.Join(rootDir, "vendor")); !os.IsNotExist(err) {
		return path.Join(rootDir, "vendor", "autoload.php"), nil
	}

	composerJSON, err := os.ReadFile(path.Join(rootDir, "composer.json"))
	if err != nil {
		return "", fmt.Errorf("cannot read composer.json: %w", err)
	}

	dependencies, err := validationComposerJSON(composerJSON)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(append(append(dependencies, 0), checkAgainst...))
	cacheRoot := path.Join(system.GetShopwareCliCacheDir(), "validate-vendor")
	workDir := path.Join(cacheRoot, hex.EncodeToString(hash[:])[:16])
	autoloadFile := path.Join(workDir, "vendor", "autoload.php")

	if info, err := os.Stat(autoloadFile); err == nil && time.Since(info.ModTime()) < validationVendorTTL {
		logging.FromContext(ctx).Debugf("Using cached composer dependencies from %s", workDir)
		return autoloadFile, nil
	}

	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", fmt.Errorf("cannot create composer cache directory: %w", err)
	}

	// Build next to the final directory and rename it into place, so parallel
	// runs never see a half-installed vendor directory.
	buildDir, err := os.MkdirTemp(cacheRoot, "build-*")
	if err != nil {
		return "", fmt.Errorf("cannot create composer cache directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	if err := os.WriteFile(path.Join(buildDir, "composer.json"), dependencies, 0o644); err != nil {
		return "", fmt.Errorf("cannot write composer.json: %w", err)
	}

	if err := runComposerDeps(ctx, buildDir, path.Join(rootDir, "auth.json"), nil, checkAgainst); err != nil {
		return "", err
	}

	if err := os.RemoveAll(workDir); err != nil {
		return "", fmt.Errorf("cannot replace cached composer dependencies: %w", err)
	}

	if err := os.Rename(buildDir, workDir); err != nil {
		// Another run finished the same dependencies first.
		if _, statErr := os.Stat(autoloadFile); statErr == nil {
			return autoloadFile, nil
		}
		return "", fmt.Errorf("cannot store composer dependencies: %w", err)
	}

	return autoloadFile, nil
}

// validationComposerJSON keeps only what decides which packages get installed,
// so extensions with the same dependencies share one cached vendor directory.
// The extension's own classes need no autoloading: PHPStan discovers them in
// the analysed paths.
func validationComposerJSON(composerJSON []byte) ([]byte, error) {
	var data map[string]any
	if err := json.Unmarshal(composerJSON, &data); err != nil {
		return nil, fmt.Errorf("cannot parse composer.json: %w", err)
	}

	dependencies := map[string]any{}
	for _, key := range []string{"require", "require-dev", "conflict", "provide", "replace", "repositories", "minimum-stability", "prefer-stable"} {
		if value, ok := data[key]; ok {
			dependencies[key] = value
		}
	}

	// Suggested packages are installed as well, so code using them can be analysed.
	if suggest, ok := data["suggest"].(map[string]any); ok && len(suggest) > 0 {
		require, _ := dependencies["require"].(map[string]any)
		if require == nil {
			require = map[string]any{}
		}
		for name := range suggest {
			if _, exists := require[name]; !exists {
				require[name] = "*"
			}
		}
		dependencies["require"] = require
	}

	// encoding/json sorts map keys, so equal dependencies give equal bytes.
	return json.Marshal(dependencies)
}

func runComposerDeps(ctx context.Context, dir string, authFile string, suggets []string, checkAgainst string) error {
	composerAuth, err := shop.ReadComposerAuth(authFile)
	if err != nil {
		return fmt.Errorf("failed to read composer auth file: %w", err)
	}

	encoded, err := composerAuth.Json(false)
	if err != nil {
		return fmt.Errorf("failed to encode composer auth: %w", err)
	}

	env := append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))

	if len(suggets) > 0 {
		additionalParams := []string{"require", "--prefer-dist", "--no-interaction", "--no-progress", "--no-plugins", "--no-scripts", "--ignore-platform-reqs"}
		for _, suggest := range suggets {
			additionalParams = append(additionalParams, suggest+":*")
		}

		composerInstall := CommandContext(ctx, "composer", additionalParams...)
		composerInstall.Env = env
		composerInstall.Dir = dir

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

	composerInstall := CommandContext(ctx, "composer", additionalParams...)
	composerInstall.Env = env
	composerInstall.Dir = dir

	beforeStart := time.Now()

	log, err := composerInstall.CombinedOutput()
	if err != nil {
		if _, writeErr := os.Stderr.Write(log); writeErr != nil {
			return fmt.Errorf("failed to write error log: %w (original error: %v)", writeErr, err)
		}
		return err
	}

	logging.FromContext(ctx).Debugf("Composer dependencies installed in %s", time.Since(beforeStart).String())

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
