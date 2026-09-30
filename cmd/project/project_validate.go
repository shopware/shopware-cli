package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
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
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		reportingFormat, err := projectValidationFormat(cmd)
		if err != nil {
			return err
		}
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		noCopy, _ := cmd.Flags().GetBool("no-copy")
		localOnly, _ := cmd.Flags().GetBool("local-only")

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

		validationPath := projectPath
		if !noCopy {
			tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-project-*")
			if err != nil {
				return fmt.Errorf("cannot create temporary directory: %w", err)
			}
			defer func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					logging.FromContext(cmd.Context()).Errorf("Failed to remove temporary directory: %v", err)
				}
			}()

			if err := system.CopyFiles(cmd.Context(), projectPath, tmpDir); err != nil {
				return err
			}
			validationPath = tmpDir
		}

		toolCfg, err := verifier.GetConfigFromProject(cmd.Context(), validationPath, localOnly)
		if err != nil {
			return err
		}

		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		var gr errgroup.Group

		tools := verifier.GetToolsOf[verifier.CheckTool]()

		tools, err = tools.Only(only)
		if err != nil {
			return err
		}

		tools, err = tools.Exclude(exclude)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Check(cmd.Context(), result, *toolCfg)
			})
		}

		if err := gr.Wait(); err != nil {
			return err
		}

		filtered := result.RemoveByIdentifier(toolCfg.ValidationIgnores)

		return validation.DoCheckReport(filtered, reportingFormat, false)
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
	projectValidateCmd.PersistentFlags().String("only", "", "Run only these tools (comma-separated; available: phpstan, eslint, stylelint, storefront-twig, sw-cli)")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Skip these tools (comma-separated); with --only, each must be selected there. Available: phpstan, eslint, stylelint, storefront-twig, sw-cli")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Validate the project directory itself, not a temporary copy")
	projectValidateCmd.PersistentFlags().Bool("local-only", false, "Validate only extensions in custom/* folders")
	projectValidateCmd.MarkFlagsMutuallyExclusive("format", "reporter")
	_ = projectValidateCmd.PersistentFlags().MarkDeprecated("reporter", "use --format instead")
	_ = projectValidateCmd.PersistentFlags().MarkHidden("reporter")
}
