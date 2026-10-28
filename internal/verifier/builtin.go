package verifier

import (
	"context"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

type Builtin struct{}

func (s Builtin) Name() string {
	return "builtin"
}

func (s Builtin) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	if len(config.Extensions) == 0 {
		return ToolSummary{SkipReason: "no extensions to check"}
	}
	ignores := make([]validation.ToolConfigIgnore, 0)

	for _, ext := range config.Extensions {
		extensionCheck := NewCheck()
		extensionCheck.SetSourceRoot(ext.GetPath())
		extension.RunValidation(ctx, ext, extensionCheck, config.PHPVersion)

		// Apply ignores from extension config
		ignores = ignores[:0] // rebuild ignores for each extension
		for _, ignore := range ext.GetExtensionConfig().Validation.Ignore {
			ignores = append(ignores, validation.ToolConfigIgnore{
				Identifier: ignore.Identifier,
				Path:       ignore.Path,
				Message:    ignore.Message,
			})
		}

		if config.InputWasDirectory {
			// Add additional ignores for directory input
			ignores = append(ignores, validation.ToolConfigIgnore{
				Identifier: "zip.disallowed_file",
			})
		}

		if len(ignores) > 0 {
			extensionCheck.RemoveByIdentifier(ignores)
		}
		findings := extensionCheck.GetResults()
		for i, result := range findings {
			// Builtin validators report extension-relative paths. Rebase them
			// before merging into the project-wide check.
			if result.Path != "" && config.RootDir != "" {
				if !filepath.IsAbs(result.Path) {
					result.Path = filepath.Join(ext.GetPath(), result.Path)
				}
				findings[i].Path = validation.NormalizeSourcePath(result.Path, config.RootDir)
			}
		}
		execution.AddFindings(findings...)
	}

	return ToolSummary{}
}

func init() {
	AddTool(Builtin{})
}
