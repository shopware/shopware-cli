package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"syscall"
	"time"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

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

// prepareComposerAutoload returns the autoload file to analyse config.RootDir with.
// A project must have an existing vendor/, which is used as it is. For an
// extension the dependencies are always updated in its validation
// cache on every run, so validation never touches vendor/, composer.json or
// composer.lock.
func prepareComposerAutoload(ctx context.Context, config ToolConfig, execution *ToolExecution) (string, *ToolError) {
	if config.IsProject {
		_, err := os.Stat(path.Join(config.RootDir, "vendor"))
		if err == nil {
			return path.Join(config.RootDir, "vendor", "autoload.php"), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", &ToolError{Err: fmt.Errorf("cannot read vendor directory: %w", err)}
		}
		return "", &ToolError{Err: errors.New(`cannot use project dependencies without vendor/: run "composer install" first`)}
	}

	composerJSON, err := os.ReadFile(path.Join(config.RootDir, "composer.json"))
	if err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot read composer.json: %w", err)}
	}

	dependencies, err := validationComposerJSON(composerJSON, config.RootDir, config.CheckAgainst)
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

	// One directory per mode, so switching between them doesn't rewrite most
	// of vendor/.
	workDir := path.Join(cacheDir, "composer-"+mode)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot create validation cache directory: %w", err)}
	}

	// Runs on the same extension, e.g. from parallel agents, take turns, so two
	// composer updates never change the same vendor/ at once.
	unlock, err := lockComposerCache(ctx, workDir+".lock")
	if err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot lock validation cache: %w", err)}
	}
	defer unlock()

	if err := os.WriteFile(path.Join(workDir, "composer.json"), dependencies, 0o644); err != nil {
		return "", &ToolError{Err: fmt.Errorf("cannot write composer.json: %w", err)}
	}

	// composer update changes vendor/ in place, so only changed packages are
	// written again.
	if toolErr := runComposerDeps(ctx, workDir, path.Join(config.RootDir, "auth.json"), config.CheckAgainst, execution); toolErr != nil {
		return "", toolErr
	}

	return path.Join(workDir, "vendor", "autoload.php"), nil
}

// lockComposerCache waits until it holds an exclusive lock on lockFile and
// returns the function that releases it. The operating system drops the lock
// when the process ends, so a crashed run never blocks the next one.
func lockComposerCache(ctx context.Context, lockFile string) (func(), error) {
	file, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	waiting := false
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Closing the file releases the lock.
			return func() { _ = file.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = file.Close()
			return nil, err
		}

		if !waiting {
			logging.FromContext(ctx).Infof("Waiting for another validation of this extension to finish its composer update")
			waiting = true
		}

		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
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

// validationComposerJSON returns the composer.json for the validation cache
// of the extension in rootDir. Autoload paths are relative to the extension
// folder and would point nowhere from the cache, so they are dropped: PHPStan
// discovers the extension's own classes in the analysed paths.
func validationComposerJSON(composerJSON []byte, rootDir string, checkAgainst string) ([]byte, error) {
	var data map[string]any
	if err := json.Unmarshal(composerJSON, &data); err != nil {
		return nil, fmt.Errorf("cannot parse composer.json: %w", err)
	}

	delete(data, "autoload")
	delete(data, "autoload-dev")

	if checkAgainst == "lowest" {
		// Composer 2.9 uses this setting; newer versions are also covered by
		// COMPOSER_POLICY_ADVISORIES_BLOCK below. Only the validation copy changes.
		config, _ := data["config"].(map[string]any)
		if config == nil {
			config = map[string]any{}
		}
		audit, _ := config["audit"].(map[string]any)
		if audit == nil {
			audit = map[string]any{}
		}
		audit["block-insecure"] = false
		config["audit"] = audit
		data["config"] = config
	}

	// Path repositories are relative to composer.json as well, but local
	// packages are still needed, so they point to the extension folder instead.
	switch repositories := data["repositories"].(type) {
	case []any:
		for _, repository := range repositories {
			resolvePathRepository(repository, rootDir)
		}
	case map[string]any:
		for _, repository := range repositories {
			resolvePathRepository(repository, rootDir)
		}
	}

	// Suggested packages are installed as well, so code using them can be analysed.
	if suggest, ok := data["suggest"].(map[string]any); ok && len(suggest) > 0 {
		require, _ := data["require"].(map[string]any)
		if require == nil {
			require = map[string]any{}
		}
		for name := range suggest {
			if _, exists := require[name]; !exists {
				require[name] = "*"
			}
		}
		data["require"] = require
	}

	return json.Marshal(data)
}

// resolvePathRepository makes the url of a relative path repository absolute.
func resolvePathRepository(repository any, rootDir string) {
	entry, ok := repository.(map[string]any)
	if !ok || entry["type"] != "path" {
		return
	}

	if url, ok := entry["url"].(string); ok && !path.IsAbs(url) {
		entry["url"] = path.Join(rootDir, url)
	}
}

func runComposerDeps(ctx context.Context, dir string, authFile string, checkAgainst string, execution *ToolExecution) *ToolError {
	composerAuth, err := shop.ReadComposerAuth(authFile)
	if err != nil {
		return &ToolError{Err: fmt.Errorf("cannot read Composer auth file: %w", err)}
	}

	encoded, err := composerAuth.Json(false)
	if err != nil {
		return &ToolError{Err: fmt.Errorf("cannot encode Composer auth: %w", err)}
	}

	env := append(os.Environ(), fmt.Sprintf("COMPOSER_AUTH=%s", encoded))
	if checkAgainst == "lowest" {
		// Historical compatibility analysis must include releases with advisories.
		// Keep audit reporting and other dependency policies enabled.
		env = append(env, "COMPOSER_POLICY_ADVISORIES_BLOCK=0")
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
