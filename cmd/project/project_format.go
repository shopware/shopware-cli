package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectFormatCmd = &cobra.Command{
	Use:   "format [path]",
	Short: "Format a project's code with PHP-CS-Fixer and Prettier",
	Long:  "Format the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed. PHP-CS-Fixer uses the project's .php-cs-fixer.dist.php if present; Prettier always uses the CLI's own config. Use --dry-run to only report files that would change.",
	Args:  cobra.MaximumNArgs(1),
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

		tools := verifier.GetToolsOf[verifier.FormatTool]()

		tools, err = tools.Only(only)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Format(cmd.Context(), *toolCfg, dryRun)
			})
		}

		return gr.Wait()
	},
}

func init() {
	projectRootCmd.AddCommand(projectFormatCmd)
	projectFormatCmd.PersistentFlags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	projectFormatCmd.PersistentFlags().Bool("dry-run", false, "Report files that would change, without changing them")
}
