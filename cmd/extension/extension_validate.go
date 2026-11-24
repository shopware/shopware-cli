package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deprecation"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var extensionValidateCmd = &cobra.Command{
	Use:   "validate path",
	Short: "Validate extension metadata, assets, and code quality",
	Long: `Validate an extension folder or ZIP file.
Prettier and PHP-CS-Fixer check formatting without changing source files. Formatting violations fail validation.
Use --exclude-formatters to skip formatting checks.

Store compliance checks that Administration and Storefront source files have matching compiled assets.
Enable it with validation.store_compliance in the extension configuration or SHOPWARE_CLI_STORE_COMPLIANCE=1.
A folder is validated in place. PHPStan resolves Composer dependencies inside var/cache/shopware-cli for the requested baseline.
The extension's vendor/ and composer.lock are preserved. Custom PHPStan configurations that load vendor/autoload.php themselves still load local dependencies.

Use --check-against next-major to check for upcoming breaking changes with PHPStan.
The rules read breaking-change attributes from the same Shopware version resolved by --check-against highest.
Shopware introduced these attributes in 6.7.14.0. Older versions may produce few or no next-major findings.
If your PHPStan config already includes future-compatibility.neon, use that config with the default mode to avoid including the ruleset twice.`,
	Example: `  # Validate an extension
  shopware-cli extension validate ./my-extension

  # Check for announced breaking changes in the next Shopware major
  shopware-cli extension validate ./my-extension --check-against next-major --only phpstan

  # Apply formatting automatically
  shopware-cli extension format ./my-extension

  # Validate without formatting checks
  shopware-cli extension validate ./my-extension --exclude-formatters`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storeCompliance, _ := cmd.Flags().GetBool("store-compliance")
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := extensionValidationFormat(cmd)
		ctx := cmd.Context()
		progress := verifier.NewProgress(reportingFormat)
		checkAgainst, _ := cmd.Flags().GetString("check-against")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		excludeFormatters, _ := cmd.Flags().GetBool("exclude-formatters")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectExtensionValidationTools(only, exclude, excludeFormatters)
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
			ext, err := extension.GetExtensionByFolder(ctx, path)
			if err != nil {
				return err
			}

			finishConfig := progress.Start("discovering extension configuration")
			toolCfg, err = verifier.ConvertExtensionToToolConfig(ctx, ext)
			finishConfig(err)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
		} else {
			finishMetadata := progress.Start("extracting extension archive")
			ext, err := extension.GetExtensionByZip(ctx, args[0])
			finishMetadata(err)
			if err != nil {
				return err
			}

			finishConfig := progress.Start("discovering extension configuration")
			toolCfg, err = verifier.ConvertExtensionToToolConfig(ctx, ext)
			finishConfig(err)
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

		if needsTools {
			finishSetup := progress.Start("preparing tools")
			setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
			finishSetup(setupErr)
			if setupErr != nil {
				return setupErr
			}
			toolCfg.ToolDirectory = verifier.GetToolDirectory()
		}

		results := verifier.RunTools(ctx, tools, statuses, *toolCfg, progress, func(tool verifier.CheckTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Check(ctx, *toolCfg, execution)
		})
		return validation.ReportTools(results, reportingFormat, "Checkers")
	},
}

func requiresToolSetup(tool verifier.CheckTool) bool {
	switch tool.(type) {
	case verifier.PhpStan, verifier.Eslint, verifier.StyleLint, verifier.Prettier, verifier.PHPCSFixer:
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
	checkAgainstModes := []string{verifier.CheckAgainstHighest, verifier.CheckAgainstLowest, verifier.CheckAgainstNextMajor}
	checkAgainstValues := strings.Join(checkAgainstModes, ", ")
	extensionRootCmd.AddCommand(extensionValidateCmd)
	extensionValidateCmd.PersistentFlags().Bool("full", false, "Run all validation checks")
	extensionValidateCmd.PersistentFlags().Bool("store-compliance", false, "Enable Store compliance (deprecated: use validation.store_compliance in the extension configuration)")
	extensionValidateCmd.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("check-against", verifier.CheckAgainstHighest, "Check Shopware compatibility ("+checkAgainstValues+")")
	extensionValidateCmd.PersistentFlags().String("only", "", "Run only these validation checks (comma-separated, e.g. phpstan,eslint)")
	extensionValidateCmd.PersistentFlags().Bool("exclude-formatters", false, "Exclude validation checks that also support formatting")
	extensionValidateCmd.PersistentFlags().String("exclude", "", "Exclude specified validation checks from running (comma-separated, e.g. phpstan,eslint). When combined with --only, exclude validation checks from the selected set")
	extensionValidateCmd.PersistentFlags().Bool("no-copy", false, "Do not copy extension files to temporary directory")
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions(validation.ReporterFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("check-against", cobra.FixedCompletions(checkAgainstModes, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.CheckTool])
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.CheckTool])
	extensionValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("full", deprecation.Message("2027-02-10", ""))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("store-compliance", deprecation.Message("2027-02-10", "validation.store_compliance in the extension configuration"))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("reporter", deprecation.Message("2027-02-10", "--format"))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("no-copy", deprecation.Message("2027-02-10", ""))
	_ = extensionValidateCmd.PersistentFlags().MarkHidden("reporter")
	extensionValidateCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if _, err := extensionValidationFormat(cmd); err != nil {
			return err
		}

		mode, _ := cmd.Flags().GetString("check-against")
		if !slices.Contains(checkAgainstModes, mode) {
			return fmt.Errorf("cannot use %q for --check-against, allowed values are %s", mode, checkAgainstValues)
		}

		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		excludeFormatters, _ := cmd.Flags().GetBool("exclude-formatters")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectExtensionValidationTools(only, exclude, excludeFormatters)
		return err
	}
}
