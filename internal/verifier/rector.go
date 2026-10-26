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

func (r Rector) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}

	_, err := os.Stat(path.Join(config.RootDir, "vendor"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot read vendor directory: %w", err)})
		return ToolSummary{}
	}
	vendorExists := err == nil

	var backupData []byte
	var composerLockData []byte

	if !vendorExists {
		composerJSONPath := path.Join(config.RootDir, "composer.json")

		backupData, err = os.ReadFile(composerJSONPath)
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot back up composer.json: %w", err)})
			return ToolSummary{}
		}
		composerLockPath := path.Join(config.RootDir, "composer.lock")
		composerLockData, err = os.ReadFile(composerLockPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot back up composer.lock: %w", err)})
			return ToolSummary{}
		}

		defer func() {
			if backupData != nil {
				if err := os.WriteFile(path.Join(config.RootDir, "composer.json"), backupData, 0o644); err != nil {
					execution.Fail(&ToolError{Err: fmt.Errorf("cannot restore composer.json: %w", err)})
				}
			}
			if composerLockData != nil {
				if err := os.WriteFile(composerLockPath, composerLockData, 0o644); err != nil {
					execution.Fail(&ToolError{Err: fmt.Errorf("cannot restore composer.lock: %w", err)})
				}
			} else if err := os.Remove(composerLockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				execution.Fail(&ToolError{Err: fmt.Errorf("cannot remove temporary Composer lock: %w", err)})
			}
		}()
		if composerLockData != nil {
			if err := os.Remove(composerLockPath); err != nil {
				execution.Fail(&ToolError{Err: fmt.Errorf("cannot remove Composer lock: %w", err)})
				return ToolSummary{}
			}
		}

		if err := installComposerDeps(ctx, config.RootDir, "highest", execution); err != nil {
			execution.Fail(err)
			return ToolSummary{}
		}
	}

	var rectorConfigFile string

	if _, err := os.Stat(path.Join(config.RootDir, "rector.php")); err == nil {
		rectorConfigFile = path.Join(config.RootDir, "rector.php")
	} else {
		if len(config.MinShopwareVersion) < 3 {
			execution.Fail(&ToolError{Err: errors.New("cannot select rector configuration: invalid Shopware version")})
			return ToolSummary{}
		}
		rectorConfigFile = path.Join(config.ToolDirectory, "php", "vendor", "frosh", "shopware-rector", "config", fmt.Sprintf("shopware-%s.0.php", config.MinShopwareVersion[0:3]))
	}

	for _, sourceDirectory := range config.SourceDirectories {
		rector := CommandContext(ctx, "php", "-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "rector"), "process", "--config", rectorConfigFile, "--autoload-file", path.Join("vendor", "autoload.php"), sourceDirectory)
		rector.Dir = config.RootDir
		if subprocess := execution.Run(rector); subprocess.Err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run rector in %s: %w", sourceDirectory, subprocess.Err)))
		}
	}

	return ToolSummary{}
}

func init() {
	AddTool(Rector{})
}
