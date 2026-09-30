package verifier

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
	"github.com/shopware/shopware-cli/logging"
)

type AdminTwigLinter struct{}

func (a AdminTwigLinter) Name() string {
	return "admin-twig"
}

func (a AdminTwigLinter) Check(ctx context.Context, check *Check, config ToolConfig) error {
	for _, p := range config.AdminDirectories {
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			if filepath.Ext(path) != twiglinter.TwigExtension {
				return nil
			}

			file, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			relPath := validation.NormalizeSourcePath(path, config.RootDir)

			_, err = html.NewAdminParser(string(file))
			if err != nil {
				line := 0
				var pe *html.ParseError
				if errors.As(err, &pe) {
					line = pe.Pos.Line
				}
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Message:    fmt.Sprintf("Failed to parse %s: %v. Create a GitHub issue with the file content.", relPath, err),
					Severity:   validation.SeverityWarning,
					Identifier: "could-not-parse-twig",
					Line:       line,
				})

				return nil
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// Fix is a no-op: Administration component migrations are no longer supported.
func (a AdminTwigLinter) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (a AdminTwigLinter) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	for _, p := range config.AdminDirectories {
		err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			if filepath.Ext(path) != twiglinter.TwigExtension {
				return nil
			}

			file, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			parsed, err := html.NewAdminParser(string(file))
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}

			if dryRun {
				if string(file) != parsed.Dump(0) {
					logging.FromContext(ctx).Infof("File %s is not correctly formatted", validation.NormalizeSourcePath(path, config.RootDir))
				}

				return nil
			} else {
				return os.WriteFile(path, []byte(parsed.Dump(0)), 0o644)
			}
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func init() {
	AddTool(AdminTwigLinter{})
}
