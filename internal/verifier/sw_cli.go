package verifier

import (
	"context"

	"github.com/shopware/shopware-cli/extension"
)

type SWCLI struct{}

func (s SWCLI) Name() string {
	return "sw-cli"
}

func (s SWCLI) Check(ctx context.Context, check *Check, config ToolConfig) error {
	if config.Extension == nil {
		return nil
	}

	validationContext := extension.RunValidation(ctx, config.Extension)

	if config.InputWasDirectory {
		validationContext.ApplyIgnores([]extension.ConfigValidationIgnoreItem{
			{
				Identifier: "zip.disallowed_file",
			},
		})
	}

	for _, err := range validationContext.Errors() {
		check.AddResult(CheckResult{
			Path:       "",
			Line:       0,
			Message:    err.Message,
			Identifier: err.Identifier,
			Severity:   "error",
		})
	}

	for _, err := range validationContext.Warnings() {
		check.AddResult(CheckResult{
			Path:       "",
			Line:       0,
			Message:    err.Message,
			Identifier: err.Identifier,
			Severity:   "warning",
		})
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
