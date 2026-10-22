package verifier

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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

func (s StorefrontTwigLinter) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolResult {
	shopwareVersion, err := version.NewVersion(config.MinShopwareVersion)
	if err != nil {
		execution.Fail(&ToolError{Err: fmt.Errorf("cannot parse Shopware version: %w", err)})
		return ToolResult{}
	}
	fixers := twiglinter.GetStorefrontFixers(shopwareVersion)
	found := false

	for _, p := range config.SourceDirectories {
		twigDir := filepath.Join(p, "Resources", "views")

		if _, err := os.Stat(twigDir); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot read template directory: %w", err)})
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
			found = true

			file, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			relPath := validation.NormalizeSourcePath(path, config.RootDir)

			parsed, err := html.NewStorefrontParser(string(file))
			if err != nil {
				line := 0
				var pe *html.ParseError
				if errors.As(err, &pe) {
					line = pe.Pos.Line
				}
				execution.AddFindings(validation.CheckResult{
					Path:       relPath,
					Message:    fmt.Sprintf("Failed to parse %s: %v. Create a GitHub issue with the file content.", relPath, err),
					Severity:   validation.SeverityWarning,
					Identifier: "could-not-parse-twig",
					Line:       line,
				})

				return nil
			}

			for _, fixer := range fixers {
				for _, message := range fixer.Check(parsed.Nodes) {
					execution.AddFindings(validation.CheckResult{
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
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot check Twig templates: %w", err)})
		}
	}

	if !found {
		return ToolResult{SkipReason: "no storefront Twig templates"}
	}
	return ToolResult{}
}
func init() {
	AddTool(StorefrontTwigLinter{})
}
