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
	Use:   "fix",
	Short: "Fix project",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		allowNonGit, _ := cmd.Flags().GetBool("allow-non-git")
		gitPath := filepath.Join(args[0], ".git")
		if !allowNonGit {
			if stat, err := os.Stat(gitPath); err != nil || !stat.IsDir() {
				return fmt.Errorf("provided folder is not a git repository. Use --allow-non-git flag to run anyway")
			}
		}

		var err error
		only, _ := cmd.Flags().GetString("only")

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = findClosestShopwareProject()
			if err != nil {
				return err
			}
		}

		projectPath, err = filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

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
