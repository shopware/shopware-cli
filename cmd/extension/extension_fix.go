package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
	"github.com/shopware/shopware-cli/logging"
)

var extensionFixCmd = &cobra.Command{
	Use:   "fix path",
	Short: "Apply code-quality fixes to an extension",
	Args:  cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return verifier.SetupTools(cmd.Context(), cmd.Root().Version)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		allowNonGit, _ := cmd.Flags().GetBool("allow-non-git")

		if !allowNonGit {
			if stat, err := os.Stat(filepath.Join(args[0], ".git")); err != nil || !stat.IsDir() {
				return fmt.Errorf("%s is not a git repository. Use --allow-non-git flag to run anyway", args[0])
			}
		}

		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
		if err != nil {
			return err
		}

		toolCfg, err := verifier.ConvertExtensionToToolConfig(ext)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Running fixes for Shopware version: %s", toolCfg.MinShopwareVersion)

		var gr errgroup.Group

		allTools := verifier.GetToolsOf[verifier.FixTool]()
		only, _ := cmd.Flags().GetString("only")

		tools, err := allTools.Only(only)
		if err != nil {
			return err
		}

		for _, tool := range tools {
			gr.Go(func() error {
				return tool.Fix(cmd.Context(), *toolCfg)
			})
		}

		runErr := gr.Wait()
		if err := validation.PrintToolInvocationTable(os.Stdout, "Fixers", extensionToolInvocationStatuses(allTools, tools)); err != nil {
			return err
		}
		return runErr
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionFixCmd)
	extensionFixCmd.Flags().String("only", "", "Run only specific fixers by name (comma-separated, e.g. eslint,rector)")
	extensionFixCmd.Flags().Bool("allow-non-git", false, "Allow running the fix command on non-git repositories")
}
