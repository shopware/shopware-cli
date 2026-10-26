package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func selectProjectValidationTools(only, exclude string) (
	verifier.ToolList[verifier.CheckTool],
	[]validation.ToolInvocationStatus,
	error,
) {
	return verifier.SelectTools(verifier.GetToolsOf[verifier.CheckTool](), only, exclude, "validation checks")
}

func selectProjectFixTools(only, exclude string) (
	verifier.ToolList[verifier.FixTool],
	[]validation.ToolInvocationStatus,
	error,
) {
	return verifier.SelectTools(verifier.GetToolsOf[verifier.FixTool](), only, exclude, "fixers")
}

func selectProjectFormattingTools(only, exclude string) (
	verifier.ToolList[verifier.FormatTool],
	[]validation.ToolInvocationStatus,
	error,
) {
	return verifier.SelectTools(verifier.GetToolsOf[verifier.FormatTool](), only, exclude, "formatters")
}

func completeToolNames[T verifier.Tool](_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return verifier.GetToolsOf[T]().CompleteNames(toComplete), cobra.ShellCompDirectiveNoFileComp
}
