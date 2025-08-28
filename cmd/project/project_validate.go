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
	Use:   "validate",
	Short: "Validate project",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		reportingFormat, _ := cmd.Flags().GetString("reporter")
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-project-*")
		noCopy, _ := cmd.Flags().GetBool("no-copy")
		if err != nil {
			return fmt.Errorf("cannot create temporary directory: %w", err)
		}

		projectPath := ""

		if len(args) > 0 {
			projectPath = args[0]
		} else {
			projectPath, err = findClosestShopwareProject()
			if err != nil {
				return err
			}
		}

		projectPath, err = filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		if reportingFormat == "" {
			reportingFormat = validation.DetectDefaultReporter()
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

		toolCfg, err := verifier.GetConfigFromProject(tmpDir)
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

func init() {
	projectRootCmd.AddCommand(projectValidateCmd)
	projectValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	projectValidateCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().String("exclude", "", "Exclude specific tools by name (comma-separated, e.g. phpstan,eslint)")
	projectValidateCmd.PersistentFlags().Bool("no-copy", false, "Do not copy project files to temporary directory")
}
