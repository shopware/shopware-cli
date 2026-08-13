package shopwarecli

import (
	"fmt"

	"github.com/shopware/shopware-cli/cmd/account"
	"github.com/shopware/shopware-cli/cmd/extension"
	"github.com/shopware/shopware-cli/cmd/project"
	"github.com/shopware/shopware-cli/internal/bootstrap"

	"github.com/spf13/cobra"
)

func NewRootCommand() (*cobra.Command, error) {
	rootCmd := &cobra.Command{	
		Use:   "shopware-cli <command> <subcommand> [flags]",
		Short: "Shopware CLI",
		Long: "A CLI for common Shopware tasks like extension management, project setup, and account management.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Run() executes steps in necessary order, threading context through each step.
			ctx, err := bootstrap.Run(
				cmd.Context(),
				cmd,
				bootstrap.LoadConfigStep(),
				bootstrap.LoadLoggerStep(),
			)
			if err != nil {
				return fmt.Errorf("error while running pre-execution: %w", err)
			}

			// Set the context for subcommands
			cmd.SetContext(ctx)

			return nil
		},
		// Silence cobra logs, because we configure our own
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// Add subcommands
	rootCmd.AddCommand(account.NewAccountCommand())
	rootCmd.AddCommand(extension.NewExtensionCommand())
	rootCmd.AddCommand(project.NewProjectCommand())

	// Register flags
	flagSet := rootCmd.PersistentFlags()
	flagSet.BoolP("verbose", "v", false, "Enable verbose output")
	flagSet.BoolP("no-interaction", "n", false, "Disable interactive prompts")
	flagSet.BoolP("no-update-notification", "", false, "Disable update notifications")

	return rootCmd, nil
}