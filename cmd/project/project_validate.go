package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Run static analysis and Shopware checks on a project",
	Long:  "Validate the project's own code, such as extensions in custom/ and configured bundles. Packages that Composer installs into vendor/ are not validated. Runs on a temporary copy unless --no-copy is passed.",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := projectValidationFormat(cmd); err != nil {
			return err
		}
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectProjectValidationTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := projectValidationFormat(cmd)
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectProjectValidationTools(only, exclude)
		if err := verifier.SetupTools(cmd.Context(), cmd.Root().Version); err != nil {
			return err
		}
		localOnly, _ := cmd.Flags().GetBool("local-only")

		var err error
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

		// POC: validate in place; PHPStan installs missing composer deps outside the folder.
		validationPath := projectPath

		toolCfg, err := verifier.GetConfigFromProject(cmd.Context(), validationPath, localOnly)
		if err != nil {
			return err
		}

		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

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
	projectValidateCmd.PersistentFlags().String("format", "", "Report format (summary, json, github, gitlab, junit, markdown; auto-detected if unset)")
	projectValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectValidateCmd.PersistentFlags().String("only", "", "Run only these validation checks (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Exclude specified validation checks from running (comma-separated, e.g. phpstan,eslint). When combined with --only, exclude validation checks from the selected set")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Validate the project directory itself, not a temporary copy")
	projectValidateCmd.PersistentFlags().Bool("local-only", false, "Validate only extensions in custom/* folders")
	_ = projectValidateCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions(validation.ReporterFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = projectValidateCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.CheckTool])
	_ = projectValidateCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.CheckTool])
	projectValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("reporter", "use --format instead")
	_ = projectValidateCmd.PersistentFlags().MarkHidden("reporter")
}
