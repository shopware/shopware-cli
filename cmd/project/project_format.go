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

var projectFormatCmd = &cobra.Command{
	Use:   "format [path]",
	Short: "Format a project's code with PHP-CS-Fixer and Prettier",
	Long:  "Format the project's own code, such as extensions in custom/ and configured bundles, and change the files directly. Packages that Composer installs into vendor/ are not changed. PHP-CS-Fixer uses the project's .php-cs-fixer.dist.php if present; Prettier always uses the CLI's own config. Use --dry-run to only report files that would change.",
	Args:  cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := projectValidationFormat(cmd); err != nil {
			return err
		}

		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		verifier.WarnOnDeprecatedToolName(cmd.Context(), only, exclude)
		_, _, err := selectProjectFormattingTools(only, exclude)
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reporting format was validated in PreRunE.
		reportingFormat, _ := projectValidationFormat(cmd)
		ctx := cmd.Context()
		progress := verifier.NewProgress(reportingFormat)
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")

		// Tool selection was validated in PreRunE.
		tools, statuses, _ := selectProjectFormattingTools(only, exclude)
		finishSetup := progress.Start("preparing tools")
		setupErr := verifier.SetupTools(ctx, cmd.Root().Version)
		finishSetup(setupErr)
		if setupErr != nil {
			return setupErr
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")

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

		finishConfig := progress.Start("discovering project configuration")
		toolCfg, err := verifier.GetConfigFromProject(ctx, projectPath, false)
		finishConfig(err)
		if err != nil {
			return err
		}

		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		var gr errgroup.Group
		for _, tool := range tools {
			gr.Go(func() error {
				finish := progress.Start(tool.Name())
				err := tool.Format(ctx, result, *toolCfg, dryRun)
				if err == nil {
					err = ctx.Err()
				}
				finish(err)
				return err
			})
		}
		runErr := gr.Wait()
		if runErr == nil {
			runErr = ctx.Err()
		}

		reportErr := validation.DoCheckReport(
			result.RemoveByIdentifier(toolCfg.ValidationIgnores),
			reportingFormat,
			runErr != nil,
			statuses...)
		if runErr != nil {
			return runErr
		}

		return reportErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectFormatCmd)
	projectFormatCmd.PersistentFlags().String("format", "", "Report format (summary, json, github, gitlab, junit, markdown; auto-detected if unset)")
	projectFormatCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectFormatCmd.PersistentFlags().String("only", "", "Run only the specified formatters (comma-separated, e.g. prettier,php-cs-fixer)")
	projectFormatCmd.PersistentFlags().Bool("dry-run", false, "Report files that would change, without changing them")
	projectFormatCmd.PersistentFlags().String("exclude", "", "Exclude specified formatters from running (comma-separated, e.g. prettier,php-cs-fixer). When combined with --only, exclude formatters from the selected set")
	_ = projectFormatCmd.RegisterFlagCompletionFunc("only", completeToolNames[verifier.FormatTool])
	_ = projectFormatCmd.RegisterFlagCompletionFunc("exclude", completeToolNames[verifier.FormatTool])
}
