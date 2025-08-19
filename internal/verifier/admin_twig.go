package verifier

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"

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
	fixers := twiglinter.GetAdministrationFixers(version.Must(version.NewVersion(config.MinShopwareVersion)))

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

			for _, fixer := range fixers {
				for _, message := range fixer.Check(parsed.Nodes) {
					check.AddResult(validation.CheckResult{
						Message:    message.Message,
						Path:       strings.TrimPrefix(strings.TrimPrefix(path, "/private"), config.RootDir+"/"),
						Line:       0,
						Severity:   message.Severity,
						Identifier: fmt.Sprintf("admintwiglinter/%s", message.Identifier),
					})
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (a AdminTwigLinter) Fix(ctx context.Context, config ToolConfig) error {
	fixers := twiglinter.GetAdministrationFixers(version.Must(version.NewVersion(config.MinShopwareVersion)))

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
				return err
			}

			for _, fixer := range fixers {
				if err := fixer.Fix(parsed.Nodes); err != nil {
					return err
				}
			}

			return os.WriteFile(path, []byte(parsed.Dump(0)), os.ModePerm)
		})
		if err != nil {
			return err
		}
	}

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
					logging.FromContext(ctx).Infof("File %s is not correctly formatted", strings.TrimPrefix(strings.TrimPrefix(path, "/private"), config.RootDir+"/"))
				}

				return nil
			} else {
				return os.WriteFile(path, []byte(parsed.Dump(0)), os.ModePerm)
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
