package verifier

import (
	"context"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

type SWCLI struct{}

func (s SWCLI) Name() string {
	return "sw-cli"
}

func (s SWCLI) Check(ctx context.Context, check *Check, config ToolConfig) error {
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

func (s SWCLI) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (s SWCLI) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

func init() {
	AddTool(SWCLI{})
}
