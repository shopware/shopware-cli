package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var projectFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Apply code-quality fixes to a project",
	Long:  "Run code-quality fixers on the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed. Requires a Git repository so the changes can be reviewed, unless --allow-non-git is passed.",
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

		allowNonGit, _ := cmd.Flags().GetBool("allow-non-git")
		if !allowNonGit {
			if stat, err := os.Stat(filepath.Join(projectPath, ".git")); err != nil || !stat.IsDir() {
				return fmt.Errorf("%s is not a git repository. Use --allow-non-git flag to run anyway", projectPath)
			}
		}

		finishConfig := progress.Start("discovering project configuration")
		toolCfg, err := verifier.GetConfigFromProject(ctx, projectPath, false)
		finishConfig(err)
		if err != nil {
			return err
		}

		results := verifier.RunTools(tools, statuses, *toolCfg, func(tool verifier.FixTool) verifier.ToolResult {
			finish := progress.Start(tool.Name())
			result := tool.Fix(ctx, *toolCfg)
			finish(result.Err)
			return result
		})
		return validation.ReportTools(results, "summary", "Fixers", logging.IsVerbose(ctx))
	},
}

func init() {
	projectRootCmd.AddCommand(projectFixCmd)
	projectFixCmd.PersistentFlags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	projectFixCmd.PersistentFlags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	projectFixCmd.PersistentFlags().String("exclude", "", "Exclude specified fixers from running (comma-separated, e.g. eslint,rector). When combined with --only, exclude fixers from the selected set")
	_ = projectFixCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FixTool])
	_ = projectFixCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FixTool])
}
