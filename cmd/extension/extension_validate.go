package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a Extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		isFull, _ := cmd.Flags().GetBool("full")
		reportingFormat, _ := cmd.Flags().GetString("reporter")
		checkAgainst, _ := cmd.Flags().GetString("check-against")
		tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-extension-*")
		only, _ := cmd.Flags().GetString("only")

		// If the user does not want to run full validation, only run shopware-cli
		if !isFull {
			only = "sw-cli"
		}

		if reportingFormat == "" {
			reportingFormat = verifier.DetectDefaultReporter()
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
			if isFull {
				if err := system.CopyFiles(args[0], tmpDir); err != nil {
					return err
				}

				defer func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						logging.FromContext(cmd.Context()).Error("Failed to remove temporary directory:", err)
					}
				}()
			} else {
				tmpDir = args[0]
			}

			ext, err := extension.GetExtensionByFolder(tmpDir)
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
		} else {
			ext, err := extension.GetExtensionByZip(args[0])
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext)
			if err != nil {
				return err
			}
		}

		toolCfg.CheckAgainst = checkAgainst
		result := verifier.NewCheck()

		var gr errgroup.Group

		tools := verifier.GetTools()

		tools, err = tools.Only(only)
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

		return verifier.DoCheckReport(result.RemoveByIdentifier(toolCfg.ValidationIgnores), reportingFormat)
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionValidateCmd)
	extensionValidateCmd.PersistentFlags().Bool("full", false, "Run full validation including PHPStan, ESLint and Stylelint")
	extensionValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("check-against", "highest", "Check against Shopware Version (highest, lowest)")
	extensionValidateCmd.PersistentFlags().String("only", "", "Run only specific tools by name (comma-separated, e.g. phpstan,eslint)")
	extensionValidateCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		reporter, _ := cmd.Flags().GetString("reporter")
		if reporter != "summary" && reporter != "json" && reporter != "github" && reporter != "junit" && reporter != "markdown" && reporter != "" {
			return fmt.Errorf("invalid reporter format: %s. Must be either 'summary', 'json', 'github', 'junit' or 'markdown'", reporter)
		}

		mode, _ := cmd.Flags().GetString("check-against")
		if mode != "highest" && mode != "lowest" {
			return fmt.Errorf("invalid mode: %s. Must be either 'highest' or 'lowest'", mode)
		}

		// Dont setup tools if we dont run full validation
		full, _ := cmd.Flags().GetBool("full")
		if !full {
			return nil
		}

		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	}
}
