package verifier

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
)

type Rector struct{}

func (r Rector) Name() string {
	return "rector"
}

func (r Rector) Check(ctx context.Context, check *Check, config ToolConfig) error {
	return nil
}

func (r Rector) Fix(ctx context.Context, config ToolConfig) error {
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		//nolint: nilerr
		return nil
	}

	_, err := os.Stat(path.Join(config.RootDir, "vendor"))
	vendorExists := !os.IsNotExist(err)

	var backupData []byte
	var composerLockData []byte

	if !vendorExists {
		composerJSONPath := path.Join(config.RootDir, "composer.json")

		if _, err := os.Stat(composerJSONPath); err == nil {
			backupData, err = os.ReadFile(composerJSONPath)
			if err != nil {
				return fmt.Errorf("failed to backup composer.json: %w", err)
			}
		}

		composerLockPath := path.Join(config.RootDir, "composer.lock")
		if _, err := os.Stat(composerLockPath); err == nil {
			composerLockData, err = os.ReadFile(composerLockPath)
			if err != nil {
				return fmt.Errorf("failed to backup composer.lock: %w", err)
			}
		}

		if _, err := os.Stat(composerLockPath); err == nil {
			if err := os.Remove(composerLockPath); err != nil {
				return err
			}
		}

		if err := installComposerDeps(ctx, config.RootDir, "highest"); err != nil {
			return err
		}
	}

	var rectorConfigFile string

	if _, err := os.Stat(path.Join(config.RootDir, "rector.php")); err == nil {
		rectorConfigFile = path.Join(config.RootDir, "rector.php")
	} else {
		rectorConfigFile = path.Join(config.ToolDirectory, "php", "vendor", "frosh", "shopware-rector", "config", fmt.Sprintf("shopware-%s.0.php", config.MinShopwareVersion[0:3]))
	}

	for _, sourceDirectory := range config.SourceDirectories {
		rector := exec.CommandContext(ctx, "php", "-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "rector"), "process", "--config", rectorConfigFile, "--autoload-file", path.Join("vendor", "autoload.php"), sourceDirectory)
		rector.Dir = config.RootDir

		log, _ := rector.CombinedOutput()
		//nolint: forbidigo
		fmt.Print(string(log))
	}

	if !vendorExists {
		if backupData != nil {
			if err := os.WriteFile(path.Join(config.RootDir, "composer.json"), backupData, 0o644); err != nil {
				return fmt.Errorf("failed to restore composer.json: %w", err)
			}
		}

		if composerLockData != nil {
			if err := os.WriteFile(path.Join(config.RootDir, "composer.lock"), composerLockData, 0o644); err != nil {
				return fmt.Errorf("failed to restore composer.lock: %w", err)
			}
		}
	}

	return nil
}

func (r Rector) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

func init() {
	AddTool(Rector{})
}
