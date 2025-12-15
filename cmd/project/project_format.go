package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFormatCmd = &cobra.Command{
	Use:   "format",
	Short: "Format project",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		only, _ := cmd.Flags().GetString("only")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

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
				return tool.Format(cmd.Context(), *toolCfg, dryRun)
			})
		}

		return gr.Wait()
	},
}

func init() {
	projectRootCmd.AddCommand(projectFormatCmd)
	projectFormatCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectFormatCmd.PersistentFlags().Bool("dry-run", false, "Run tools in dry run mode")
}
