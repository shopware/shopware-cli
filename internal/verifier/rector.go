package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
)

type Rector struct{}

func (r Rector) Name() string {
	return "rector"
}

func (r Rector) Fix(ctx context.Context, config ToolConfig) (result ToolResult) {
	if len(config.SourceDirectories) == 0 {
		return ToolResult{SkipReason: "no source directories"}
	}
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ToolResult{SkipReason: "no composer.json found"}
		}
		return ToolResult{Err: fmt.Errorf("cannot read composer manifest: %w", err)}
	}

	_, err := os.Stat(path.Join(config.RootDir, "vendor"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ToolResult{Err: fmt.Errorf("cannot read vendor directory: %w", err)}
	}
	vendorExists := err == nil

	var backupData []byte
	var composerLockData []byte

	if !vendorExists {
		composerJSONPath := path.Join(config.RootDir, "composer.json")

		backupData, err = os.ReadFile(composerJSONPath)
		if err != nil {
			return ToolResult{Err: fmt.Errorf("cannot back up composer.json: %w", err)}
		}
		composerLockPath := path.Join(config.RootDir, "composer.lock")
		composerLockData, err = os.ReadFile(composerLockPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ToolResult{Err: fmt.Errorf("cannot back up composer.lock: %w", err)}
		}

		defer func() {
			if backupData != nil {
				if err := os.WriteFile(path.Join(config.RootDir, "composer.json"), backupData, 0o644); err != nil {
					result.Err = errors.Join(result.Err, fmt.Errorf("cannot restore composer.json: %w", err))
				}
			}
			if composerLockData != nil {
				if err := os.WriteFile(composerLockPath, composerLockData, 0o644); err != nil {
					result.Err = errors.Join(result.Err, fmt.Errorf("cannot restore composer.lock: %w", err))
				}
			} else if err := os.Remove(composerLockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot remove temporary Composer lock: %w", err))
			}
		}()
		if composerLockData != nil {
			if err := os.Remove(composerLockPath); err != nil {
				return ToolResult{Err: fmt.Errorf("cannot remove Composer lock: %w", err)}
			}
		}

		if err := installComposerDeps(ctx, config.RootDir, "highest", &result); err != nil {
			result.Err = err
			return result
		}
	}

	var rectorConfigFile string

	if _, err := os.Stat(path.Join(config.RootDir, "rector.php")); err == nil {
		rectorConfigFile = path.Join(config.RootDir, "rector.php")
	} else {
		if len(config.MinShopwareVersion) < 3 {
			result.Err = errors.New("cannot select rector configuration: invalid Shopware version")
			return result
		}
		rectorConfigFile = path.Join(config.ToolDirectory, "php", "vendor", "frosh", "shopware-rector", "config", fmt.Sprintf("shopware-%s.0.php", config.MinShopwareVersion[0:3]))
	}

	for _, sourceDirectory := range config.SourceDirectories {
		rector := CommandContext(ctx, "php", "-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "rector"), "process", "--config", rectorConfigFile, "--autoload-file", path.Join("vendor", "autoload.php"), sourceDirectory)
		rector.Dir = config.RootDir
		if _, _, err := captureCommand(rector, &result); err != nil {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot run rector in %s: %w", sourceDirectory, err))
		}
	}

	return result
}

func init() {
	AddTool(Rector{})
}
