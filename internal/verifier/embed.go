package verifier

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

//go:embed php/composer.json php/composer.lock php/configs js/configs js/package*.json
var toolsFS embed.FS

func SetupTools(ctx context.Context, currentVersion string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	customToolDir := os.Getenv("SHOPWARE_CLI_TOOLS_DIR")
	if customToolDir != "" {
		logging.FromContext(ctx).Debugf("Using custom tool directory: %s", customToolDir)
		setToolDirectory(customToolDir)
		return nil
	}

	cacheDir := system.GetShopwareCliCacheDir()
	toolsDir := path.Join(cacheDir, "tools", currentVersion)

	if _, err := os.Stat(toolsDir); err == nil {
		logging.FromContext(ctx).Debugf("Using cached tool directory: %s", toolsDir)
		setToolDirectory(toolsDir)
		return nil
	}

	if ok, err := system.IsPHPVersionAtLeast(ctx, "8.2.0"); err != nil {
		return fmt.Errorf("failed to check installed PHP version: %w", err)
	} else if !ok {
		return errors.New("the validation tools require PHP 8.2 or newer; update PHP or run inside the shopware-cli Docker image")
	}

	if ok, err := system.IsNodeVersionAtLeast(ctx, "20.0.0"); err != nil {
		return fmt.Errorf("failed to check installed Node.js version: %w", err)
	} else if !ok {
		return errors.New("the validation tools require Node.js 20 or newer; update Node.js or run inside the shopware-cli Docker image")
	}

	logging.FromContext(ctx).Debugf("Using tool directory: %s", toolsDir)
	if err := os.MkdirAll(path.Dir(toolsDir), 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", toolsDir, err)
	}

	// Publish only complete installations. Cancellation can leave a large dependency
	// tree; keep it in the cache's temporary directory rather than blocking exit
	// on recursive deletion. Clearing the CLI cache also removes these directories.
	installDir, err := os.MkdirTemp(path.Dir(toolsDir), ".incomplete-")
	if err != nil {
		return fmt.Errorf("cannot create tool installation directory: %w", err)
	}
	defer func() {
		if ctx.Err() == nil {
			_ = os.RemoveAll(installDir)
		}
	}()

	if err := unpackFile(ctx, toolsFS, ".", installDir); err != nil {
		return fmt.Errorf("failed to unpack file: %w", err)
	}

	composerInstall := CommandContext(ctx, "composer", "install", "--no-dev")
	composerInstall.Dir = path.Join(installDir, "php")

	if output, err := composerInstall.CombinedOutput(); err != nil {
		log.Println(string(output))
		return fmt.Errorf("failed to install composer dependencies: %w", err)
	}

	jsInstall := CommandContext(ctx, "npm", "install")
	jsInstall.Dir = path.Join(installDir, "js")

	if output, err := jsInstall.CombinedOutput(); err != nil {
		log.Println(string(output))
		return fmt.Errorf("failed to install npm dependencies: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(installDir, toolsDir); err != nil {
		// Another invocation may have finished installing the same version first.
		if _, statErr := os.Stat(toolsDir); statErr != nil {
			return fmt.Errorf("cannot publish tool installation: %w", err)
		}
	}

	setToolDirectory(toolsDir)
	return nil
}

func unpackFile(ctx context.Context, fs embed.FS, filePath, unpackDir string) error {
	f, err := fs.ReadDir(filePath)
	if err != nil {
		return err
	}

	for _, file := range f {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.IsDir() {
			if err := os.MkdirAll(path.Join(unpackDir, file.Name()), 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", path.Join(unpackDir, file.Name()), err)
			}

			if err := unpackFile(ctx, fs, path.Join(filePath, file.Name()), path.Join(unpackDir, file.Name())); err != nil {
				return fmt.Errorf("failed to unpack file %s: %w", path.Join(filePath, file.Name()), err)
			}
		} else {
			content, err := fs.ReadFile(path.Join(filePath, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path.Join(filePath, file.Name()), err)
			}

			if err := os.WriteFile(path.Join(unpackDir, file.Name()), content, 0o644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", path.Join(unpackDir, file.Name()), err)
			}
		}
	}

	return nil
}
