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

var extensionFormat = &cobra.Command{
	Use:   "format path",
	Short: "Format an extension's PHP, JavaScript, and SCSS files",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := extensionValidationFormat(cmd); err != nil {
			return err
		}

		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectExtensionFormattingTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := extensionValidationFormat(cmd)
		timeout, _ := cmd.Flags().GetString("timeout")
		ctx, cancel, err := verifier.WithQATimeout(cmd.Context(), timeout, cmd.Flags().Changed("timeout"))
		if err != nil {
			return err
		}
		defer cancel()

		progress := verifier.NewProgress(reportingFormat)
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionFormattingTools(only, exclude)

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(ctx, path)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseExtensionDiscovery, err)
		if err != nil {
			return err
		}

		finishSetup := progress.Start("preparing tools")
		setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
		setupErr = verifier.QAPhaseError(ctx, verifier.QAPhaseToolSetup, setupErr)
		finishSetup(setupErr)
		if setupErr != nil {
			return setupErr
		}
		finishConfig := progress.Start("discovering extension configuration")
		toolCfg, err := verifier.ConvertExtensionToToolConfig(ctx, ext)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseConfigurationDiscovery, err)
		finishConfig(err)
		if err != nil {
			return err
		}

		logging.FromContext(ctx).Debugf("Running formatters for Shopware version: %s", toolCfg.MinShopwareVersion)

		results := verifier.RunTools(ctx, tools, statuses, *toolCfg, progress, func(tool verifier.FormatTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Format(ctx, *toolCfg, execution, dryRun)
		})
		cancel()

		results = verifier.EnsureQATimeoutResult(ctx, results)
		return validation.ReportTools(results, reportingFormat, "Formatters")
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFormat)

	extensionFormat.PersistentFlags().String("format", "", "Report format (summary, json, github, gitlab, junit, markdown; auto-detected if unset)")
	extensionFormat.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionFormat.Flags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	extensionFormat.Flags().String("exclude", "", "Exclude specified formatters from running (comma-separated, e.g. prettier,php-cs-fixer). When combined with --only, exclude formatters from the selected set")
	extensionFormat.Flags().Bool("dry-run", false, "Report files that would change, without changing them")
	extensionFormat.Flags().String("timeout", verifier.DefaultQATimeout.String(), "Limit total QA execution time (0 disables; "+verifier.QATimeoutEnv+" sets the default)")

	_ = extensionFormat.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FormatTool])
	_ = extensionFormat.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FormatTool])
}
