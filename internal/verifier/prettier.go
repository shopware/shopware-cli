package verifier

import (
	"context"
	"os"
	"os/exec"
	"path"

	"golang.org/x/sync/errgroup"
)

var ignoredPaths = `
package-lock.json
Resources/public/**
Resources/app/storefront/dist/**
dist/**
Resources/store/**
`

type Prettier struct{}

func (b Prettier) Name() string {
	return "prettier"
}

func (b Prettier) Check(ctx context.Context, check *Check, config ToolConfig) error {
	return nil
}

func (b Prettier) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (b Prettier) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	var gr errgroup.Group

	for _, sourceDirectory := range config.SourceDirectories {
		sourceDirectory := sourceDirectory

		if err := os.WriteFile(path.Join(sourceDirectory, ".prettierignore"), []byte(ignoredPaths), 0o644); err != nil {
			return err
		}

		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "prettier"),
			"--config",
			path.Join(config.ToolDirectory, "js", "configs", "prettierrc.js"),
			".",
		}

		if !dryRun {
			args = append(args, "--write")
		} else {
			args = append(args, "--check")
		}

		gr.Go(func() error {
			cmd := exec.CommandContext(ctx, "node", args...)
			cmd.Dir = sourceDirectory
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				return err
			}

			return os.Remove(path.Join(sourceDirectory, ".prettierignore"))
		})
	}

	return gr.Wait()
}

func init() {
	AddTool(Prettier{})
}
