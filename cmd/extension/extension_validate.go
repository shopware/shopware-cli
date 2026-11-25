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

Folders are validated in place. ZIP files are extracted to a temporary
folder before validation.

PHP-CS-Fixer and Prettier check formatting in dry-run mode without changing
source files. Formatting differences fail validation. Use --exclude-formatters
to skip these checks.

Store compliance checks report Administration and Storefront sources without a
build directory and builds without sources. Enable these checks with
validation.store_compliance in the extension configuration or
SHOPWARE_CLI_STORE_COMPLIANCE=1.

Use --check-against to select the lowest or highest supported Shopware version
for PHPStan. Dependencies for the analysis are installed in
var/cache/shopware-cli, leaving the extension's vendor/ and composer.lock
unchanged. A custom PHPStan configuration that loads vendor/autoload.php still
loads local dependencies.

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
		timeout, _ := cmd.Flags().GetString("timeout")
		ctx, cancel, err := verifier.WithQATimeout(cmd.Context(), timeout, cmd.Flags().Changed("timeout"))
		if err != nil {
			return err
		}
		defer cancel()

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
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseExtensionDiscovery, err)
			if err != nil {
				return err
			}

			finishConfig := progress.Start("discovering extension configuration")
			toolCfg, err = verifier.ConvertExtensionToToolConfig(ctx, ext)
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseConfigurationDiscovery, err)
			finishConfig(err)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
		} else {
			finishMetadata := progress.Start("extracting extension archive")
			ext, err := extension.GetExtensionByZip(ctx, args[0])
			if err == nil {
				defer func() { _ = os.RemoveAll(filepath.Dir(ext.GetPath())) }()
			}
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseArchiveExtraction, err)
			finishMetadata(err)
			if err != nil {
				return err
			}

			finishConfig := progress.Start("discovering extension configuration")
			toolCfg, err = verifier.ConvertExtensionToToolConfig(ctx, ext)
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseConfigurationDiscovery, err)
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
			setupErr = verifier.QAPhaseError(ctx, verifier.QAPhaseToolSetup, setupErr)
			finishSetup(setupErr)
			if setupErr != nil {
				return setupErr
			}
			toolCfg.ToolDirectory = verifier.GetToolDirectory()
		}

		results := verifier.RunTools(ctx, tools, statuses, *toolCfg, progress, func(tool verifier.CheckTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Check(ctx, *toolCfg, execution)
		})
		cancel()

		results = verifier.EnsureQATimeoutResult(ctx, results)
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
	extensionValidateCmd.Flags().String("timeout", verifier.DefaultQATimeout.String(), "Interrupt validation after e.g. 30s, 5m or 1h30m (no timeout by default, 0 disables, "+verifier.QATimeoutEnv+" sets the default)")

	extensionValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("full", deprecation.Message("2027-02-10", ""))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("store-compliance", deprecation.Message("2027-02-10", "validation.store_compliance in the extension configuration"))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("reporter", deprecation.Message("2027-02-10", "--format"))
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("no-copy", deprecation.Message("2027-02-10", ""))
	_ = extensionValidateCmd.PersistentFlags().MarkHidden("reporter")

	_ = extensionValidateCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions(validation.ReporterFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("check-against", cobra.FixedCompletions(checkAgainstModes, cobra.ShellCompDirectiveNoFileComp))
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.CheckTool])
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.CheckTool])

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
