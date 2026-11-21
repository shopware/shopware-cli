package extension

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deprecation"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionFixCmd = &cobra.Command{
	Use:   "fix path",
	Short: "Apply code-quality fixes to an extension",
	Long: `Apply code-quality fixes to an extension.

Fixes include PHP refactoring, JavaScript and SCSS corrections, and conversion
of Symfony service and routing XML in plugins to YAML.

Rector uses the extension's rector.php when present and the bundled Shopware
rules otherwise. It installs the lowest compatible dependencies in
var/cache/shopware-cli.

Changes are written directly to source files. Use --dry-run to report findings
and proposed changes without modifying files. If the command is interrupted, it
does not undo changes already made.`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectExtensionFixTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		timeout, _ := cmd.Flags().GetString("timeout")
		ctx, cancel, err := verifier.WithQATimeout(cmd.Context(), timeout, cmd.Flags().Changed("timeout"))
		if err != nil {
			return err
		}
		defer cancel()

		progress := verifier.NewProgress("")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionFixTools(only, exclude)

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(ctx, path)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseExtensionDiscovery, err)
		if err != nil {
			return err
		}

		finishSetup := progress.Start("preparing tools and discovering extension configuration")
		toolCfg, err := verifier.SetupExtensionToolConfig(ctx, cmd.Root().Version, ext)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseSetupAndDiscovery, err)
		finishSetup(err)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		results := verifier.RunTools(
			ctx, tools, statuses, *toolCfg, progress,
			func(tool verifier.FixTool, execution *verifier.ToolExecution) verifier.ToolSummary {
				return tool.Fix(ctx, *toolCfg, execution, dryRun)
			},
		)
		cancel()

		results = verifier.EnsureQATimeoutResult(ctx, results)
		return validation.ReportTools(results, "summary", "Fixers")
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFixCmd)

	extensionFixCmd.Flags().Bool("dry-run", false, "Report files that would change, without changing them")
	extensionFixCmd.Flags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	extensionFixCmd.Flags().String("exclude", "", "Exclude specified fixers from running (comma-separated, e.g. eslint,rector). When combined with --only, exclude fixers from the selected set")
	extensionFixCmd.Flags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	extensionFixCmd.Flags().String("timeout", verifier.DefaultQATimeout.String(), "Interrupt fixing after e.g. 30s, 5m or 1h30m (no timeout by default, 0 disables, "+verifier.QATimeoutEnv+" sets the default)")

	_ = extensionFixCmd.Flags().MarkDeprecated("allow-non-git", deprecation.Message("2027-02-10", ""))

	_ = extensionFixCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FixTool])
	_ = extensionFixCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FixTool])
}
