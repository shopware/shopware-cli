package verifier

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

//go:embed php/composer.json php/composer.lock php/configs js/configs js/packages js/package*.json
var toolsFS embed.FS

func SetupTools(ctx context.Context, currentVersion string) error {
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

	if ok, err := system.IsPHPVersionAtLeast("8.2.0"); err != nil {
		return fmt.Errorf("failed to check installed PHP version: %w", err)
	} else if !ok {
		return fmt.Errorf("php version must be at least 8.2.0 to use this. Update your PHP version or use the shopware-cli docker image")
	}

	if ok, err := system.IsNodeVersionAtLeast("20.0.0"); err != nil {
		return fmt.Errorf("failed to check installed Node.js version: %w", err)
	} else if !ok {
		return fmt.Errorf("node.js version must be at least 20.0.0 to use this. Update your Node.js version or use the shopware-cli docker image")
	}

	logging.FromContext(ctx).Debugf("Using tool directory: %s", toolsDir)
	if err := os.MkdirAll(toolsDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", toolsDir, err)
	}

	if err := unpackFile(toolsFS, ".", toolsDir); err != nil {
		if err := os.RemoveAll(toolsDir); err != nil {
			return err
		}
		return fmt.Errorf("failed to unpack file: %w", err)
	}

	composerInstall := exec.Command("composer", "install", "--no-dev")
	composerInstall.Dir = path.Join(toolsDir, "php")

	if output, err := composerInstall.CombinedOutput(); err != nil {
		log.Println(string(output))
		if err := os.RemoveAll(toolsDir); err != nil {
			return err
		}
		return fmt.Errorf("failed to install composer dependencies: %w", err)
	}

	jsInstall := exec.Command("npm", "install")
	jsInstall.Dir = path.Join(toolsDir, "js")

	if output, err := jsInstall.CombinedOutput(); err != nil {
		log.Println(string(output))
		if err := os.RemoveAll(toolsDir); err != nil {
			return err
		}
		return fmt.Errorf("failed to install npm dependencies: %w", err)
	}

	setToolDirectory(toolsDir)
	return nil
}

func unpackFile(fs embed.FS, filePath, unpackDir string) error {
	f, err := fs.ReadDir(filePath)
	if err != nil {
		return err
	}

	for _, file := range f {
		if file.IsDir() {
			if err := os.MkdirAll(path.Join(unpackDir, file.Name()), os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", path.Join(unpackDir, file.Name()), err)
			}

			if err := unpackFile(fs, path.Join(filePath, file.Name()), path.Join(unpackDir, file.Name())); err != nil {
				return fmt.Errorf("failed to unpack file %s: %w", path.Join(filePath, file.Name()), err)
			}
		} else {
			content, err := fs.ReadFile(path.Join(filePath, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path.Join(filePath, file.Name()), err)
			}

			if err := os.WriteFile(path.Join(unpackDir, file.Name()), content, os.ModePerm); err != nil {
				return fmt.Errorf("failed to write file %s: %w", path.Join(unpackDir, file.Name()), err)
			}
		}
	}

	return nil
}
