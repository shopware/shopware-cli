package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionFormat = &cobra.Command{
	Use:   "format path",
	Short: "Format an extension's PHP, JavaScript, SCSS, and Administration Twig files",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
		if err != nil {
			return err
		}

		toolCfg, err := verifier.ConvertExtensionToToolConfig(ext)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		var gr errgroup.Group

		allTools := verifier.GetToolsOf[verifier.FormatTool]()
		only, _ := cmd.Flags().GetString("only")

		tools, err := allTools.Only(only)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Format(cmd.Context(), *toolCfg, dryRun)
			})
		}

		runErr := gr.Wait()
		if err := validation.PrintToolInvocationTable(os.Stdout, "Formatters", extensionToolInvocationStatuses(allTools, tools)); err != nil {
			return err
		}
		return runErr
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFormat)
	extensionFormat.Flags().String("only", "", "Run only specific formatters by name (comma-separated, e.g. prettier,php-cs-fixer)")
	extensionFormat.Flags().Bool("dry-run", false, "Run in dry run mode")
}
