package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deprecation"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Apply code-quality fixes to a project",
	Long:  "Run code-quality fixers on the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed. Use --dry-run to report findings and proposed changes without applying fixes.",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectProjectFixTools(only, exclude)
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
		tools, statuses, _ := selectProjectFixTools(only, exclude)
		finishSetup := progress.Start("preparing tools")
		setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
		setupErr = verifier.QAPhaseError(ctx, verifier.QAPhaseToolSetup, setupErr)
		finishSetup(setupErr)
		if setupErr != nil {
			return setupErr
		}

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = shop.FindClosestShopwareProject(false)
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseProjectDiscovery, err)
			if err != nil {
				return err
			}
		}

		projectPath, err = filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		finishConfig := progress.Start("discovering project configuration")
		toolCfg, err := verifier.GetConfigFromProject(ctx, projectPath, false)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseConfigurationDiscovery, err)
		finishConfig(err)
		if err != nil {
			return err
		}

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
	projectRootCmd.AddCommand(projectFixCmd)

	projectFixCmd.PersistentFlags().Bool("dry-run", false, "Report files that would change, without changing them")
	projectFixCmd.PersistentFlags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	projectFixCmd.PersistentFlags().String("exclude", "", "Exclude specified fixers from running (comma-separated, e.g. eslint,rector). When combined with --only, exclude fixers from the selected set")
	projectFixCmd.PersistentFlags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	projectFixCmd.Flags().String("timeout", verifier.DefaultQATimeout.String(), "Limit total QA execution time (0 disables; "+verifier.QATimeoutEnv+" sets the default)")

	_ = projectFixCmd.PersistentFlags().MarkDeprecated("allow-non-git", deprecation.Message("2027-02-10", ""))

	_ = projectFixCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FixTool])
	_ = projectFixCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FixTool])
}
