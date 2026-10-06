package extension

import (
	"errors"
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
	Short: "Format an extension's PHP, JavaScript, and SCSS files",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		allTools := verifier.GetToolsOf[verifier.FormatTool]()
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)

		requestedTools, err := allTools.Only(only)
		if err != nil {
			return err
		}
		tools, err := requestedTools.Exclude(exclude)
		if err != nil {
			return err
		}
		if len(tools) == 0 {
			return errors.New("no formatters selected after applying --exclude")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		allTools := verifier.GetToolsOf[verifier.FormatTool]()
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		requestedTools, _ := allTools.Only(only)
		tools, _ := requestedTools.Exclude(exclude)

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
		if err != nil {
			return err
		}

		toolCfg, err := verifier.SetupExtensionToolConfig(cmd.Context(), cmd.Root().Version, ext)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		var gr errgroup.Group

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Format(cmd.Context(), *toolCfg, dryRun)
			})
		}

		runErr := gr.Wait()
		if err := validation.PrintToolInvocationTable(os.Stdout, "Formatters", extensionToolInvocationStatuses(allTools, requestedTools, tools)); err != nil {
			return err
		}
		return runErr
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFormat)
	extensionFormat.Flags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	extensionFormat.Flags().String("exclude", "", "Skip these formatters (comma-separated, e.g. prettier,php-cs-fixer); when --only is set, excluded formatters must be selected there")
	extensionFormat.Flags().Bool("dry-run", false, "Run in dry run mode")
}
