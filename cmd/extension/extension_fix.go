package extension

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionFixCmd = &cobra.Command{
	Use:   "fix path",
	Short: "Apply code-quality fixes to an extension",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectExtensionFixTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		progress := verifier.NewProgress("")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionFixTools(only, exclude)

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
		if err != nil {
			return err
		}

		finishSetup := progress.Start("preparing tools and discovering extension configuration")
		toolCfg, err := verifier.SetupExtensionToolConfig(cmd.Context(), cmd.Root().Version, ext)
		finishSetup(err)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		results := verifier.RunTools(cmd.Context(), tools, statuses, *toolCfg, progress, func(tool verifier.FixTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Fix(cmd.Context(), *toolCfg, execution)
		})
		return validation.ReportTools(results, "summary", "Fixers")
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFixCmd)
	extensionFixCmd.Flags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	extensionFixCmd.Flags().String("exclude", "", "Exclude specified fixers from running (comma-separated, e.g. eslint,rector). When combined with --only, exclude fixers from the selected set")
	extensionFixCmd.Flags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	_ = extensionFixCmd.Flags().MarkDeprecated("allow-non-git", "a Git repository is no longer required; omit --allow-non-git")
	_ = extensionFixCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FixTool])
	_ = extensionFixCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FixTool])
}
