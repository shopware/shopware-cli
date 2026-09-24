package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var projectValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Run static analysis and Shopware checks on a project",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := projectValidationFormat(cmd); err != nil {
			return err
		}
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		reportingFormat, err := projectValidationFormat(cmd)
		if err != nil {
			return err
		}
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-project-*")
		noCopy, _ := cmd.Flags().GetBool("no-copy")
		localOnly, _ := cmd.Flags().GetBool("local-only")
		targetVersion, _ := cmd.Flags().GetString("target-version")
		if err != nil {
			return fmt.Errorf("cannot create temporary directory: %w", err)
		}

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

		if !noCopy {
			if err := system.CopyFiles(projectPath, tmpDir); err != nil {
				return err
			}

			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					logging.FromContext(cmd.Context()).Errorf("Failed to remove temporary directory: %v", err)
				}
			}()
		} else {
			tmpDir = projectPath
		}

		toolCfg, err := verifier.GetConfigFromProject(cmd.Context(), tmpDir, localOnly, targetVersion)
		if err != nil {
			return err
		}

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

		filtered := result.RemoveByIdentifier(toolCfg.ValidationIgnores)

		return validation.DoCheckReport(filtered, reportingFormat)
	},
}

func projectValidationFormat(cmd *cobra.Command) (string, error) {
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
	projectRootCmd.AddCommand(projectValidateCmd)
	projectValidateCmd.PersistentFlags().String("format", "", "Validation report format (summary, json, github, gitlab, junit, or markdown)")
	projectValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectValidateCmd.PersistentFlags().String("only", "", "Run only the specified tools (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Exclude tools after applying --only; names must be in the selected set (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Validate the project in place instead of copying it to a temporary directory")
	projectValidateCmd.PersistentFlags().Bool("local-only", false, "Validate only extensions in custom/* folders")
	projectValidateCmd.PersistentFlags().String("target-version", "", "Shopware release to validate against, e.g. 6.7.14.2, or a minor like 6.7 for its newest release (default: lowest release matching the shopware/core constraint)")
	_ = projectValidateCmd.RegisterFlagCompletionFunc("target-version", func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return verifier.TargetVersionCompletions(cmd.Context()), cobra.ShellCompDirectiveNoFileComp
	})
	projectValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("reporter", "use --format instead")
	_ = projectValidateCmd.PersistentFlags().MarkHidden("reporter")
}
