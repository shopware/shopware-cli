package verifier

import (
	"context"
	"os"
	"os/exec"
	"path"

	"golang.org/x/sync/errgroup"
)

type PHPCSFixer struct{}

func (p PHPCSFixer) Name() string {
	return "php-cs-fixer"
}

func (p PHPCSFixer) Check(ctx context.Context, check *Check, config ToolConfig) error {
	return nil
}

func (p PHPCSFixer) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (p PHPCSFixer) getConfigPath(toolDirectory, rootDir string) string {
	if _, err := os.Stat(path.Join(rootDir, ".php-cs-fixer.dist.php")); err == nil {
		return path.Join(rootDir, ".php-cs-fixer.dist.php")
	}

	return path.Join(toolDirectory, "php", "configs", "php-cs-fixer.dist.php")
}

func (p PHPCSFixer) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	// Apps don't have an composer.json file, skip them
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		//nolint: nilerr
		return nil
	}

	var gr errgroup.Group

	for _, sourceDirectory := range config.SourceDirectories {
		fixDir := sourceDirectory

		args := []string{"fix", "--config", p.getConfigPath(config.ToolDirectory, config.RootDir), fixDir}
		if dryRun {
			args = append(args, "--dry-run")
		}

		cmd := exec.CommandContext(ctx, path.Join(config.ToolDirectory, "php", "vendor", "bin", "php-cs-fixer"), args...)
		cmd.Dir = config.RootDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		gr.Go(func() error {
			return cmd.Run()
		})
	}

	return gr.Wait()
}

func init() {
	AddTool(PHPCSFixer{})
}
