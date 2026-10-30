package verifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// validationVendorTTL keeps "highest" meaningful: after a day the cached
// dependencies are resolved again to pick up new releases.
const validationVendorTTL = 24 * time.Hour

// preparePHPTool checks whether a tool has PHP sources and a Composer manifest.
func preparePHPTool(config ToolConfig, execution *ToolExecution) (ToolSummary, bool) {
	if len(config.SourceDirectories) == 0 {
		return ToolSummary{SkipReason: "no source directories"}, false
	}
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ToolSummary{SkipReason: "no composer.json found"}, false
		}
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot read composer manifest: %w", err)})
		return ToolSummary{}, false
	}
	return ToolSummary{}, true
}

// installComposerDeps installs the dependencies into rootDir itself when vendor/ is missing.
func installComposerDeps(ctx context.Context, rootDir string, checkAgainst string, execution *ToolExecution) *ToolError {
	_, err := os.Stat(path.Join(rootDir, "vendor"))
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return &ToolError{Err: fmt.Errorf("cannot read vendor directory: %w", err)}
	}

	return runComposerDeps(ctx, rootDir, path.Join(rootDir, "auth.json"), getComposerSuggets(rootDir), checkAgainst, execution)
}

// prepareComposerAutoload returns the autoload file to analyse config.RootDir with.
// An existing vendor/ is used as it is. A project must have one; for an
// extension without vendor/ the dependencies are installed into its validation
// cache, so validation never touches vendor/, composer.json or composer.lock.
func prepareComposerAutoload(ctx context.Context, config ToolConfig, execution *ToolExecution) (string, *ToolError) {
	_, err := os.Stat(path.Join(config.RootDir, "vendor"))
	if err == nil {
		if config.CheckAgainst == "lowest" {
			logging.FromContext(ctx).Warnf("Using the existing vendor/ directory, --check-against lowest has no effect")
		}

		return path.Join(config.RootDir, "vendor", "autoload.php"), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", &ToolError{Err: fmt.Errorf("cannot read vendor directory: %w", err)}
	}

	if config.IsProject {
		return "", &ToolError{Err: errors.New(`cannot run phpstan without vendor/: run "composer install" first (pass --exclude phpstan to skip it)`)}
	}

	composerJSON, err := os.ReadFile(path.Join(config.RootDir, "composer.json"))
	if err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot read composer.json: %w", err)}
	}

	dependencies, err := validationComposerJSON(composerJSON)
	if err != nil {
		return "", &ToolError{Err: err}
	}

	cacheDir, err := ensureValidationCacheDir(config.RootDir)
	if err != nil {
		return "", &ToolError{Err: err}
	}

	mode := "highest"
	if config.CheckAgainst == "lowest" {
		mode = "lowest"
	}

	workDir := path.Join(cacheDir, "composer-"+mode)
	autoloadFile := path.Join(workDir, "vendor", "autoload.php")

	if isComposerCacheFresh(workDir, dependencies) {
		logging.FromContext(ctx).Debugf("Using cached composer dependencies from %s", workDir)
		return autoloadFile, nil
	}

	// Build next to the final directory and rename it into place, so parallel
	// runs never see a half-installed vendor directory.
	buildDir, err := os.MkdirTemp(cacheDir, "build-*")
	if err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot create composer cache directory: %w", err)}
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	if err := os.WriteFile(path.Join(buildDir, "composer.json"), dependencies, 0o644); err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot write composer.json: %w", err)}
	}

	if toolErr := runComposerDeps(ctx, buildDir, path.Join(config.RootDir, "auth.json"), nil, config.CheckAgainst, execution); toolErr != nil {
		return "", toolErr
	}

	if err := os.RemoveAll(workDir); err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot replace cached composer dependencies: %w", err)}
	}

	if err := os.Rename(buildDir, workDir); err != nil {
		// Another run finished the same dependencies first.
		if _, statErr := os.Stat(autoloadFile); statErr == nil {
			return autoloadFile, nil
		}
		return "", &ToolError{Err: fmt.Errorf("cannot store composer dependencies: %w", err)}
	}

	return autoloadFile, nil
}

// isComposerCacheFresh reports whether workDir holds an install of exactly
// these dependencies that is younger than validationVendorTTL.
func isComposerCacheFresh(workDir string, dependencies []byte) bool {
	cached, err := os.ReadFile(path.Join(workDir, "composer.json"))
	if err != nil || !bytes.Equal(cached, dependencies) {
		return false
	}

	info, err := os.Stat(path.Join(workDir, "vendor", "autoload.php"))

	return err == nil && time.Since(info.ModTime()) < validationVendorTTL
}

// ensureValidationCacheDir creates the validation cache of rootDir. The cache
// ignores itself in git, so the user's .gitignore stays untouched.
func ensureValidationCacheDir(rootDir string) (string, error) {
	cacheDir := path.Join(rootDir, extension.ValidationCacheDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create validation cache directory: %w", err)
	}

	gitignore := path.Join(cacheDir, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		if err := os.WriteFile(gitignore, []byte("*\n"), 0o644); err != nil {
			return "", fmt.Errorf("cannot create validation cache directory: %w", err)
		}
	}

	return cacheDir, nil
}

// validationComposerJSON keeps only what decides which packages get installed,
// so the cached install is only redone when that changes.
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

func runComposerDeps(ctx context.Context, dir string, authFile string, suggets []string, checkAgainst string, execution *ToolExecution) *ToolError {
	composerAuth, err := shop.ReadComposerAuth(authFile)
	if err != nil {
		return &ToolError{Err: fmt.Errorf("cannot read Composer auth file: %w", err)}
	}

	encoded, err := composerAuth.Json(false)
	if err != nil {
		return &ToolError{Err: fmt.Errorf("cannot encode Composer auth: %w", err)}
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

		subprocess := execution.Run(OperationComposerRequire, composerInstall)
		if subprocess.Err != nil {
			return subprocess.ToolError(fmt.Errorf("cannot install Composer suggestions: %w", subprocess.Err))
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

	subprocess := execution.Run(OperationComposerUpdate, composerInstall)
	if subprocess.Err != nil {
		return subprocess.ToolError(fmt.Errorf("cannot install Composer dependencies: %w", subprocess.Err))
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
