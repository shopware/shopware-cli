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

var projectFormatCmd = &cobra.Command{
	Use:   "format [path]",
	Short: "Run configured formatters on project files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)

		tools, statuses, err := selectProjectTools(verifier.GetToolsOf[verifier.FormatTool](), only, exclude, "formatters")
		if err != nil {
			return err
		}
		if err := verifier.SetupTools(cmd.Context(), cmd.Root().Version); err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")

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

		toolCfg, err := verifier.GetConfigFromProject(cmd.Context(), projectPath, false)
		if err != nil {
			return err
		}

		var gr errgroup.Group

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Format(cmd.Context(), *toolCfg, dryRun)
			})
		}

		runErr := gr.Wait()
		if err := validation.PrintToolInvocationTable(os.Stdout, "Formatters", statuses); err != nil {
			return err
		}
		return runErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectFormatCmd)
	projectFormatCmd.PersistentFlags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	projectFormatCmd.PersistentFlags().String("exclude", "", "Exclude formatters after applying --only (comma-separated, e.g. prettier,php-cs-fixer)")
	projectFormatCmd.PersistentFlags().Bool("dry-run", false, "Run formatters without changing files")
}
