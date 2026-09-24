package extension

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		isFull, _ := cmd.Flags().GetBool("full")
		storeCompliance, _ := cmd.Flags().GetBool("store-compliance")
		reportingFormat, err := extensionValidationFormat(cmd)
		if err != nil {
			return err
		}
		checkAgainst, _ := cmd.Flags().GetString("check-against")
		targetVersion, _ := cmd.Flags().GetString("target-version")
		tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-extension-*")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		noCopy, _ := cmd.Flags().GetBool("no-copy")

		// If the user does not want to run full validation, only run shopware-cli
		if !isFull {
			only = "sw-cli"
		}

		if err != nil {
			return fmt.Errorf("cannot create temporary directory: %w", err)
		}

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
			if noCopy {
				tmpDir = path
				logging.FromContext(cmd.Context()).Debugf("Skipping copying extension files to temporary directory due to --no-copy flag")
			} else if isFull {
				beforeCopyTime := time.Now()

				// The copy gets the target release installed, so the existing vendor/ must not travel along
				var skip []string
				if targetVersion != "" {
					skip = verifier.TargetCopySkips
				}

				if err := system.CopyFiles(args[0], tmpDir, skip...); err != nil {
					return err
				}

				logging.FromContext(cmd.Context()).Debugf("Copied extension files to temporary directory in %s", time.Since(beforeCopyTime).String())

				defer func() {
					beforeDeleteTime := time.Now()
					if err := os.RemoveAll(tmpDir); err != nil {
						logging.FromContext(cmd.Context()).Errorf("Failed to remove temporary directory: %v", err)
					}
					logging.FromContext(cmd.Context()).Debugf("Removed temporary directory in %s", time.Since(beforeDeleteTime).String())
				}()
			} else if !isFull {
				tmpDir = args[0]
			}

			ext, err := extension.GetExtensionByFolder(cmd.Context(), tmpDir)
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext, targetVersion)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
			toolCfg.RootDirIsCopy = isFull && !noCopy
		} else {
			ext, err := extension.GetExtensionByZip(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext, targetVersion)
			if err != nil {
				return err
			}

			toolCfg.RootDirIsCopy = true
		}

		if storeCompliance || os.Getenv("SHOPWARE_CLI_STORE_COMPLIANCE") == "1" {
			toolCfg.Extension.GetExtensionConfig().Validation.StoreCompliance = true
			// The user is not allowed to provide a custom ignore list when store compliance is enabled
			toolCfg.Extension.GetExtensionConfig().Validation.Ignore = extension.ConfigValidationList{}
		}

		// Without --full only metadata checks run, and none of them read the Shopware version
		if !isFull {
			toolCfg.Target = validation.Target{}
		}

		toolCfg.CheckAgainst = checkAgainst
		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		tools := verifier.GetTools()

		tools, err = tools.Only(only)
		if err != nil {
			return err
		}

		tools, err = tools.Exclude(exclude)
		if err != nil {
			return err
		}

		if err := tools.RunChecks(cmd.Context(), result, *toolCfg); err != nil {
			return err
		}

		return validation.DoCheckReport(result.RemoveByIdentifier(toolCfg.ValidationIgnores), reportingFormat)
	},
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
	extensionValidateCmd.PersistentFlags().Bool("full", false, "Run full validation including PHPStan, ESLint and Stylelint")
	extensionValidateCmd.PersistentFlags().Bool("store-compliance", false, "Run the Extension Store compliance checks")
	extensionValidateCmd.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("check-against", "highest", "Check against Shopware Version (highest, lowest)")
	extensionValidateCmd.PersistentFlags().String("target-version", "", "Shopware release to validate against, e.g. 6.7.14.2, or a minor like 6.7 for its newest release (default: lowest release matching the shopware/core constraint)")
	_ = extensionValidateCmd.RegisterFlagCompletionFunc("target-version", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return verifier.TargetVersionCompletions(cmd.Context()), cobra.ShellCompDirectiveNoFileComp
	})
	extensionValidateCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	extensionValidateCmd.PersistentFlags().String("exclude", "", "Exclude specific tools by name (comma-separated, e.g. phpstan,eslint)")
	extensionValidateCmd.PersistentFlags().Bool("no-copy", false, "Do not copy extension files to temporary directory")
	extensionValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
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

		full, _ := cmd.Flags().GetBool("full")
		targetVersion, _ := cmd.Flags().GetString("target-version")
		if targetVersion != "" && !full {
			return errors.New("--target-version needs --full. Without --full only metadata checks run, and none of them depend on the Shopware version")
		}

		// Dont setup tools if we dont run full validation
		if !full {
			return nil
		}

		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	}
}
