package extension

import (
	"errors"
	"fmt"
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
		// Reporting format and tool selection were validated in PreRunE.
		reportingFormat, _ := extensionValidationFormat(cmd)
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionFormattingTools(only, exclude)

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

		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		var gr errgroup.Group
		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Format(cmd.Context(), result, *toolCfg, dryRun)
			})
		}
		runErr := gr.Wait()

		reportErr := validation.DoCheckReport(result.RemoveByIdentifier(
			toolCfg.ValidationIgnores),
			reportingFormat,
			runErr != nil,
			statuses...)
		if runErr != nil {
			return runErr
		}

		return reportErr
	},
}

func selectExtensionFormattingTools(only, exclude string) (verifier.ToolList[verifier.FormatTool], []validation.ToolInvocationStatus, error) {
	validationTools := verifier.GetToolsOf[verifier.FormatTool]()

	requestedTools, err := validationTools.Only(only)
	if err != nil {
		return nil, nil, err
	}
	selected, err := requestedTools.Exclude(exclude)
	if err != nil {
		return nil, nil, err
	}
	if len(selected) == 0 {
		return nil, nil, errors.New("no format tools selected after applying --exclude")
	}

	return selected, extensionToolInvocationStatuses(validationTools, requestedTools, selected), nil
}

func init() {
	extensionRootCmd.AddCommand(extensionFormat)
	extensionFormat.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionFormat.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionFormat.Flags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	extensionFormat.Flags().String("exclude", "", "Exclude specified formatters from running (comma-separated, e.g. prettier,php-cs-fixer). When combined with --only, exclude formatters from the selected set")
	extensionFormat.Flags().Bool("dry-run", false, "Run in dry run mode")
	_ = extensionFormat.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FormatTool])
	_ = extensionFormat.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FormatTool])
}
