package extension

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionFormat = &cobra.Command{
	Use:   "format",
	Short: "Format an extension",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(path)
		if err != nil {
			return err
		}

		toolCfg, err := verifier.ConvertExtensionToToolConfig(ext)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		var gr errgroup.Group

		tools := verifier.GetTools()
		only, _ := cmd.Flags().GetString("only")

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

		if err := gr.Wait(); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFormat)
	extensionFormat.Flags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	extensionFormat.Flags().Bool("dry-run", false, "Run in dry run mode")
}
