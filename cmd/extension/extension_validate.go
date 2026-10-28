package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var extensionValidateCmd = &cobra.Command{
	Use:   "validate path",
	Short: "Validate extension metadata, assets, and code quality",
	Long:  "Validate an extension folder or ZIP file. With --store-compliance (or SHOPWARE_CLI_STORE_COMPLIANCE=1), the Store's rules apply and the extension's validation.ignore list is not used.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storeCompliance, _ := cmd.Flags().GetBool("store-compliance")
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := extensionValidationFormat(cmd)
		checkAgainst, _ := cmd.Flags().GetString("check-against")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionValidationTools(only, exclude)
		needsTools := slices.ContainsFunc(tools, requiresToolSetup)

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}
		var toolCfg *verifier.ToolConfig

		if stat.IsDir() {
			// POC: validate in place; PHPStan installs missing composer deps outside the folder.
			validationPath := path

			ext, err := extension.GetExtensionByFolder(cmd.Context(), validationPath)
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(cmd.Context(), ext)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
		} else {
			ext, err := extension.GetExtensionByZip(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(cmd.Context(), ext)
			if err != nil {
				return err
			}
		}

		if storeCompliance || os.Getenv("SHOPWARE_CLI_STORE_COMPLIANCE") == "1" {
			toolCfg.Extensions[0].GetExtensionConfig().Validation.StoreCompliance = true
			// The user is not allowed to provide a custom ignore list when store compliance is enabled
			toolCfg.Extensions[0].GetExtensionConfig().Validation.Ignore = extension.ConfigValidationList{}
			toolCfg.ValidationIgnores = nil
		}

		toolCfg.CheckAgainst = checkAgainst
		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		if needsTools {
			if err := verifier.SetupTools(cmd.Context(), cmd.Root().Version); err != nil {
				return err
			}
			toolCfg.ToolDirectory = verifier.GetToolDirectory()
		}

		var gr errgroup.Group
		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Check(cmd.Context(), result, *toolCfg)
			})
		}

		runErr := gr.Wait()
		reportErr := validation.DoCheckReport(result.RemoveByIdentifier(toolCfg.ValidationIgnores), reportingFormat, runErr != nil, statuses...)
		if runErr != nil {
			return runErr
		}
		return reportErr
	},
}

func requiresToolSetup(tool verifier.CheckTool) bool {
	switch tool.(type) {
	case verifier.PhpStan, verifier.Eslint, verifier.StyleLint:
		return true
	default:
		return false
	}
}

func extensionValidationFormat(cmd *cobra.Command) (string, error) {
	format, _ := cmd.Flags().GetString("format")
	reporter, _ := cmd.Flags().GetString("reporter")
	if reporter != "" {
		format = reporter
	}
	if format == "" {
		format = validation.DetectDefaultReporter()
	}

	return format, validation.ValidateReporter(format)
}

func init() {
	extensionRootCmd.AddCommand(extensionValidateCmd)
	extensionValidateCmd.PersistentFlags().Bool("full", false, "Run all validation checks")
	extensionValidateCmd.PersistentFlags().Bool("store-compliance", false, "Run the Extension Store compliance checks")
	extensionValidateCmd.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("check-against", "highest", "Check against Shopware Version (highest, lowest)")
	extensionValidateCmd.PersistentFlags().String("only", "", "Run only these validation checks (comma-separated, e.g. phpstan,eslint)")
	extensionValidateCmd.PersistentFlags().String("exclude", "", "Exclude specified validation checks from running (comma-separated, e.g. phpstan,eslint). When combined with --only, exclude validation checks from the selected set")
	extensionValidateCmd.PersistentFlags().Bool("no-copy", false, "Do not copy extension files to temporary directory")
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions(validation.ReporterFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("check-against", cobra.FixedCompletions([]string{"highest", "lowest"}, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.CheckTool])
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.CheckTool])
	extensionValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("full", "all validation checks now run by default; omit --full; to restore old behaviour use --only builtin")
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("reporter", "use --format instead")
	_ = extensionValidateCmd.PersistentFlags().MarkHidden("reporter")
	extensionValidateCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if _, err := extensionValidationFormat(cmd); err != nil {
			return err
		}

		mode, _ := cmd.Flags().GetString("check-against")
		if mode != "highest" && mode != "lowest" {
			return fmt.Errorf("invalid --check-against value %q, allowed values: highest, lowest", mode)
		}

		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectExtensionValidationTools(only, exclude)
		return err
	}
}
