package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Apply code-quality fixes to a project",
	Long:  "Run code-quality fixers on the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed.",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectProjectFixTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		progress := verifier.NewProgress("")

		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectProjectFixTools(only, exclude)
		finishSetup := progress.Start("preparing tools")
		setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
		finishSetup(setupErr)
		if setupErr != nil {
			return setupErr
		}

		var err error
		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = shop.FindClosestShopwareProject(false)
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
		finishConfig(err)
		if err != nil {
			return err
		}

		results := verifier.RunTools(ctx, tools, statuses, *toolCfg, progress, func(tool verifier.FixTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Fix(ctx, *toolCfg, execution)
		})
		return validation.ReportTools(results, "summary", "Fixers")
	},
}

func init() {
	projectRootCmd.AddCommand(projectFixCmd)
	projectFixCmd.PersistentFlags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	projectFixCmd.PersistentFlags().String("exclude", "", "Exclude specified fixers from running (comma-separated, e.g. eslint,rector). When combined with --only, exclude fixers from the selected set")
	projectFixCmd.PersistentFlags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	_ = projectFixCmd.PersistentFlags().MarkDeprecated("allow-non-git", "a Git repository is no longer required; omit --allow-non-git")
	_ = projectFixCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FixTool])
	_ = projectFixCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FixTool])
}
