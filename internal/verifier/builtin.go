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

func (s Builtin) Check(ctx context.Context, check *Check, config ToolConfig) error {
	ignores := make([]validation.ToolConfigIgnore, 0)
	ctx = extension.WithProjectPHPVersion(ctx, config.PHPVersion)

	for _, ext := range config.Extensions {
		extensionCheck := NewCheck()
		extensionCheck.SetSourceRoot(ext.GetPath())
		extension.RunValidation(ctx, ext, extensionCheck)

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
		for _, result := range extensionCheck.GetResults() {
			// Builtin validators report extension-relative paths. Rebase them
			// before merging into the project-wide check.
			if result.Path != "" && config.RootDir != "" {
				if !filepath.IsAbs(result.Path) {
					result.Path = filepath.Join(ext.GetPath(), result.Path)
				}
				result.Path = validation.NormalizeSourcePath(result.Path, config.RootDir)
			}
			check.AddResult(result)
		}
	}

	return nil
}

func init() {
	AddTool(Builtin{})
}
