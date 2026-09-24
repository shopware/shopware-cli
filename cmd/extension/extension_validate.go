package extension

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

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
		only, _ := cmd.Flags().GetString("only")
		exclude, _ := cmd.Flags().GetString("exclude")
		noCopy, _ := cmd.Flags().GetBool("no-copy")

		tools, coverage, err := selectExtensionValidationTools(isFull, only, exclude)
		if err != nil {
			return err
		}
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
				logging.FromContext(cmd.Context()).Debugf("Skipping copying extension files to temporary directory due to --no-copy flag")
			} else if needsTools {
				tmpDir, err := os.MkdirTemp(os.TempDir(), "analyse-extension-*")
				if err != nil {
					return fmt.Errorf("cannot create temporary directory: %w", err)
				}
				defer func() {
					beforeDeleteTime := time.Now()
					if err := os.RemoveAll(tmpDir); err != nil {
						logging.FromContext(cmd.Context()).Errorf("Failed to remove temporary directory: %v", err)
					}
					logging.FromContext(cmd.Context()).Debugf("Removed temporary directory in %s", time.Since(beforeDeleteTime).String())
				}()

				beforeCopyTime := time.Now()
				if err := system.CopyFiles(path, tmpDir); err != nil {
					return err
				}

				logging.FromContext(cmd.Context()).Debugf("Copied extension files to temporary directory in %s", time.Since(beforeCopyTime).String())
				validationPath = tmpDir
			}

			ext, err := extension.GetExtensionByFolder(cmd.Context(), validationPath)
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext)
			if err != nil {
				return err
			}

			toolCfg.InputWasDirectory = true
		} else {
			ext, err := extension.GetExtensionByZip(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			toolCfg, err = verifier.ConvertExtensionToToolConfig(ext)
			if err != nil {
				return err
			}
		}

		if storeCompliance || os.Getenv("SHOPWARE_CLI_STORE_COMPLIANCE") == "1" {
			toolCfg.Extension.GetExtensionConfig().Validation.StoreCompliance = true
			// The user is not allowed to provide a custom ignore list when store compliance is enabled
			toolCfg.Extension.GetExtensionConfig().Validation.Ignore = extension.ConfigValidationList{}
		}

		toolCfg.CheckAgainst = checkAgainst
		result := verifier.NewCheck()
		result.SetSourceRoot(toolCfg.RootDir)

		if needsTools {
			if err := verifier.SetupTools(cmd.Context(), cmd.Root().Version); err != nil {
				return err
			}
			toolCfg.ToolDirectory = verifier.GetToolDirectory()
		}

		var gr errgroup.Group
		for _, tool := range tools {
			checker := tool.(verifier.CheckTool)
			gr.Go(func() error {
				return checker.Check(cmd.Context(), result, *toolCfg)
			})
		}

		if err := gr.Wait(); err != nil {
			return err
		}

		return validation.DoCheckReport(result.RemoveByIdentifier(toolCfg.ValidationIgnores), reportingFormat, coverage...)
	},
}

func selectExtensionValidationTools(full bool, only, exclude string) (verifier.ToolList, []validation.CheckCoverage, error) {
	validationTools := verifier.GetToolsOf[verifier.CheckTool]()

	requested := only
	if requested == "" {
		requested = "sw-cli"
		if full {
			requested = validationTools.PossibleString()
		}
	}
	selected, err := verifier.GetTools().Only(requested)
	if err != nil {
		return nil, nil, err
	}
	requestedNames := make(map[string]bool, len(selected))
	for _, tool := range selected {
		name := tool.Name()
		if _, ok := tool.(verifier.CheckTool); !ok {
			return nil, nil, fmt.Errorf("%s does not provide a validation check", name)
		}
		requestedNames[name] = true
	}
	selected, err = selected.Exclude(exclude)
	if err != nil {
		return nil, nil, err
	}
	if len(selected) == 0 {
		return nil, nil, errors.New("no validation checks selected after applying --exclude")
	}

	unique := make(verifier.ToolList, 0, len(selected))
	invoked := make(map[string]bool, len(selected))
	for _, tool := range selected {
		if invoked[tool.Name()] {
			continue
		}
		unique = append(unique, tool)
		invoked[tool.Name()] = true
	}

	coverage := make([]validation.CheckCoverage, 0, len(validationTools))
	for _, tool := range validationTools {
		name := tool.Name()
		check := validation.CheckCoverage{Name: name, Status: "skipped"}
		switch {
		case invoked[name]:
			check.Status = "invoked"
		case requestedNames[name]:
			check.Reason = "excluded by --exclude"
		case only != "":
			check.Reason = "not selected by --only"
		default:
			check.Reason = "not selected; use --full or --only"
		}
		coverage = append(coverage, check)
	}
	sort.Slice(coverage, func(i, j int) bool { return coverage[i].Name < coverage[j].Name })
	return unique, coverage, nil
}

func requiresToolSetup(tool verifier.Tool) bool {
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
	extensionValidateCmd.PersistentFlags().Bool("full", false, "Run all validation checks by default (minus --exclude selections)")
	extensionValidateCmd.PersistentFlags().Bool("store-compliance", false, "Run the Extension Store compliance checks")
	extensionValidateCmd.PersistentFlags().String("format", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("reporter", "", "Reporting format (summary, json, github, gitlab, junit, markdown)")
	extensionValidateCmd.PersistentFlags().String("check-against", "highest", "Check against Shopware Version (highest, lowest)")
	extensionValidateCmd.PersistentFlags().String("only", "", "Run only these validation checks, regardless of --full (comma-separated, e.g. phpstan,eslint)")
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

		return nil
	}
}
