package project

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deprecation"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

var projectValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Run static analysis and Shopware checks on a project",
	Long: `Validate project extensions and configured bundles.

Extensions outside vendor/ are checked, including those in custom/. Packages
installed in vendor/ are not checked. PHPStan uses the project's installed
dependencies, so run composer install first.

PHP-CS-Fixer and Prettier check formatting in dry-run mode without changing
source files. Formatting differences fail validation. Use --exclude-formatters
to skip these checks.`,
	Example: `  # Validate a project
  shopware-cli project validate ./my-project

  # Apply formatting automatically
  shopware-cli project format ./my-project

  # Validate without formatting checks
  shopware-cli project validate ./my-project --exclude-formatters`,
	Args: cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := projectValidationFormat(cmd); err != nil {
			return err
		}
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		excludeFormatters, _ := cmd.Flags().GetBool("exclude-formatters")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectProjectValidationTools(only, exclude, excludeFormatters)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := projectValidationFormat(cmd)
		timeout, _ := cmd.Flags().GetString("timeout")
		ctx, cancel, err := verifier.WithQATimeout(cmd.Context(), timeout, cmd.Flags().Changed("timeout"))
		if err != nil {
			return err
		}
		defer cancel()

		progress := verifier.NewProgress(reportingFormat)
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		excludeFormatters, _ := cmd.Flags().GetBool("exclude-formatters")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectProjectValidationTools(only, exclude, excludeFormatters)
		localOnly, _ := cmd.Flags().GetBool("local-only")

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = shop.FindClosestShopwareProject(false)
			err = verifier.QAPhaseError(ctx, verifier.QAPhaseProjectDiscovery, err)
			if err != nil {
				return err
			}
		}

		projectPath, err = filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		finishConfig := progress.Start("discovering project configuration")
		toolCfg, err := verifier.GetConfigFromProject(ctx, projectPath, localOnly)
		err = verifier.QAPhaseError(ctx, verifier.QAPhaseConfigurationDiscovery, err)
		finishConfig(err)
		if err != nil {
			return err
		}

		finishSetup := progress.Start("preparing tools")
		setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
		setupErr = verifier.QAPhaseError(ctx, verifier.QAPhaseToolSetup, setupErr)
		finishSetup(setupErr)
		if setupErr != nil {
			return setupErr
		}
		toolCfg.ToolDirectory = verifier.GetToolDirectory()

		results := verifier.RunTools(ctx, tools, statuses, *toolCfg, progress, func(tool verifier.CheckTool, execution *verifier.ToolExecution) verifier.ToolSummary {
			return tool.Check(ctx, *toolCfg, execution)
		})
		cancel()

		results = verifier.EnsureQATimeoutResult(ctx, results)
		return validation.ReportTools(results, reportingFormat, "Checkers")
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
	projectValidateCmd.PersistentFlags().Bool("exclude-formatters", false, "Exclude validation checks that also support formatting")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Exclude specified validation checks from running (comma-separated, e.g. phpstan,eslint). When combined with --only, exclude validation checks from the selected set")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Validate the project directory itself, not a temporary copy")
	projectValidateCmd.PersistentFlags().Bool("local-only", false, "Validate only extensions in custom/* folders")
	projectValidateCmd.Flags().String("timeout", verifier.DefaultQATimeout.String(), "Interrupt validation after e.g. 30s, 5m or 1h30m (no timeout by default, 0 disables, "+verifier.QATimeoutEnv+" sets the default)")

	projectValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("reporter", deprecation.Message("2027-02-10", "--format"))
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("no-copy", deprecation.Message("2027-02-10", ""))
	_ = projectValidateCmd.PersistentFlags().MarkHidden("reporter")

	_ = projectValidateCmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions(validation.ReporterFormats, cobra.ShellCompDirectiveNoFileComp))
	_ = projectValidateCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.CheckTool])
	_ = projectValidateCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.CheckTool])
}
