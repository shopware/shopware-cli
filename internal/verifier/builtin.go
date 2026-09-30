package verifier

import (
	"context"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

type Builtin struct{}

func (s Builtin) Name() string {
	return "builtin"
}

func (s Builtin) Check(ctx context.Context, check *Check, config ToolConfig) error {
	if config.Extension == nil {
		return nil
	}

	extension.RunValidation(ctx, config.Extension, check)

	// Apply ignores from extension config
	ignores := make([]validation.ToolConfigIgnore, 0)
	for _, ignore := range config.Extension.GetExtensionConfig().Validation.Ignore {
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
		check.RemoveByIdentifier(ignores)
	}

	return nil
}

func init() {
	AddTool(Builtin{})
}
