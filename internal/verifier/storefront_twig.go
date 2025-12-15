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
	_ "github.com/shopware/shopware-cli/internal/verifier/twiglinter/storefronttwiglinter"
)

type StorefrontTwigLinter struct{}

func (s StorefrontTwigLinter) Name() string {
	return "storefront-twig"
}

func (s StorefrontTwigLinter) Check(ctx context.Context, check *Check, config ToolConfig) error {
	fixers := twiglinter.GetStorefrontFixers(version.Must(version.NewVersion(config.MinShopwareVersion)))

	for _, p := range config.SourceDirectories {
		twigDir := filepath.Join(p, "Resources", "views")

		if _, err := os.Stat(twigDir); err != nil {
			continue
		}

		err := filepath.WalkDir(twigDir, func(path string, d fs.DirEntry, err error) error {
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

			relPath := strings.TrimPrefix(strings.TrimPrefix(path, "/private"), config.RootDir+"/")

			parsed, err := html.NewStorefrontParser(string(file))
			if err != nil {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Message:    fmt.Sprintf("Failed to parse %s: %v. Create a GitHub issue with the file content.", path, err),
					Severity:   validation.SeverityWarning,
					Identifier: "could-not-parse-twig",
					Line:       0,
				})

				return nil
			}

			for _, fixer := range fixers {
				for _, message := range fixer.Check(parsed.Nodes) {
					check.AddResult(validation.CheckResult{
						Path:       relPath,
						Message:    message.Message,
						Severity:   message.Severity,
						Identifier: message.Identifier,
						Line:       message.Line,
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
func (s StorefrontTwigLinter) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (a StorefrontTwigLinter) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

func init() {
	AddTool(StorefrontTwigLinter{})
}
