package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Apply code-quality fixes to a project",
	Long:  "Run code-quality fixers on the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed. Requires a Git repository so the changes can be reviewed, unless --allow-non-git is passed.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)

		tools, statuses, err := selectProjectTools(verifier.GetToolsOf[verifier.FixTool](), only, exclude, "fixers")
		if err != nil {
			return err
		}
		if err := verifier.SetupTools(cmd.Context(), cmd.Root().Version); err != nil {
			return err
		}

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

		toolCfg, err := verifier.GetConfigFromProject(cmd.Context(), projectPath, false)
		if err != nil {
			return err
		}

		var gr errgroup.Group

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Fix(cmd.Context(), *toolCfg)
			})
		}

		runErr := gr.Wait()
		if err := validation.PrintToolInvocationTable(os.Stdout, "Fixers", statuses); err != nil {
			return err
		}
		return runErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectFixCmd)
	projectFixCmd.PersistentFlags().String("only", "", "Run only the specified fixers (comma-separated, e.g. eslint,rector)")
	projectFixCmd.PersistentFlags().Bool("allow-non-git", false, "Allow fix to run outside a Git repository")
	projectFixCmd.PersistentFlags().String("exclude", "", "Exclude fixers after applying --only (comma-separated, e.g. eslint,rector)")
}
