package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var projectValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate project",
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
		if err != nil {
			return fmt.Errorf("cannot create temporary directory: %w", err)
		}

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = findClosestShopwareProject(false)
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
					logging.FromContext(cmd.Context()).Error("Failed to remove temporary directory:", err)
				}
			}()
		} else {
			tmpDir = projectPath
		}

		toolCfg, err := verifier.GetConfigFromProject(tmpDir, localOnly)
		if err != nil {
			return err
		}

		result := verifier.NewCheck()

		var gr errgroup.Group

		tools := verifier.GetTools()

		tools, err = tools.Only(only)
		if err != nil {
			return err
		}

		tools, err = tools.Exclude(exclude)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			tool := tool
			gr.Go(func() error {
				return tool.Check(cmd.Context(), result, *toolCfg)
			})
		}

		if err := gr.Wait(); err != nil {
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
	projectValidateCmd.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectValidateCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Exclude specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Do not copy project files to temporary directory")
	projectValidateCmd.PersistentFlags().Bool("local-only", false, "Only read plugins in custom/* folders")
	projectValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("reporter", "use --format instead")
	_ = projectValidateCmd.PersistentFlags().MarkHidden("reporter")
}
