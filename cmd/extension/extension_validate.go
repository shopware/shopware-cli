package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionValidateCmd = &cobra.Command{
	Use:   "validate path",
	Short: "Validate extension metadata, assets, and code quality",
	Long: `Validate an extension folder or ZIP file.
Store compliance checks that Administration and Storefront source files have matching compiled assets.
Enable it with validation.store_compliance in the extension configuration or SHOPWARE_CLI_STORE_COMPLIANCE=1.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		storeCompliance, _ := cmd.Flags().GetBool("store-compliance")
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := extensionValidationFormat(cmd)
		ctx := cmd.Context()
		progress := verifier.NewProgress(reportingFormat)
		cleanup := func() {}
		defer func() { cleanup() }()
		logger := logging.FromContext(ctx)
		checkAgainst, _ := cmd.Flags().GetString("check-against")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		noCopy, _ := cmd.Flags().GetBool("no-copy")

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
			validationPath := path
			if noCopy {
				logger.Debugf("Skipping copying extension files to temporary directory due to --no-copy flag")
			} else if needsTools {
				tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-extension-*")
				if err != nil {
					return fmt.Errorf("cannot create temporary directory: %w", err)
				}
				cleanup = sync.OnceFunc(func() {
					finishCleanup := progress.Start("removing temporary files")
					cleanupErr := os.RemoveAll(tmpDir)
					finishCleanup(cleanupErr)
					if cleanupErr != nil {
						logger.Errorf("Failed to remove temporary directory: %v", cleanupErr)
					}
				})

				beforeCopyTime := time.Now()
				finishCopy := progress.Start("copying files (use \"--no-copy\" to skip)")
				copyErr := system.CopyFiles(ctx, path, tmpDir)
				finishCopy(copyErr)
				if copyErr != nil {
					return copyErr
				}

				logger.Debugf("Copied extension files to temporary directory in %s", time.Since(beforeCopyTime).String())
				validationPath = tmpDir
			}

			ext, err := extension.GetExtensionByFolder(ctx, validationPath)
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
		cleanup()
		return validation.ReportTools(results, reportingFormat, "Checkers")
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
	extensionValidateCmd.PersistentFlags().Bool("store-compliance", false, "Enable Store compliance (deprecated: use validation.store_compliance in the extension configuration)")
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
	_ = extensionValidateCmd.PersistentFlags().MarkDeprecated("store-compliance", "use validation.store_compliance in the extension configuration")
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
