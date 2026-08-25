package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFixCmd = &cobra.Command{
	Use:   "fix [path]",
	Short: "Fix project",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = findClosestShopwareProject(false)
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

		only, _ := cmd.Flags().GetString("only")

		toolCfg, err := verifier.GetConfigFromProject(projectPath, false)
		if err != nil {
			return err
		}

		var gr errgroup.Group

		tools := verifier.GetTools()

		tools, err = tools.Only(only)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			tool := tool
			gr.Go(func() error {
				return tool.Fix(cmd.Context(), *toolCfg)
			})
		}

		return gr.Wait()
	},
}

func init() {
	projectRootCmd.AddCommand(projectFixCmd)
	projectFixCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectFixCmd.PersistentFlags().Bool("allow-non-git", false, "Allow running on non git repositories")
}
