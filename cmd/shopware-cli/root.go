package shopwarecli

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/cmd/account"
	"github.com/shopware/shopware-cli/cmd/extension"
	"github.com/shopware/shopware-cli/cmd/project"
	"github.com/shopware/shopware-cli/internal/bootstrap"
	"github.com/shopware/shopware-cli/internal/tracking"

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
				// ?? bootstrap.LoadUpdateCheckerStep(),
			)
			if err != nil {
				return fmt.Errorf("error while running pre-execution: %w", err)
			}

			// Create context that reacts to ctrlc and sigterm

			// --verbose? store in context

			// not in terminal? interactive=false

			// make cli version available in context

			// set shopware account api user agent

			// find executed command

			// Set the context for subcommands
			cmd.SetContext(ctx)

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			
			// was successful? store in context

			// make telemetry friendly format of command name
			name := strings.TrimPrefix(cmd.CommandPath(), "shopware-cli ")
			name = strings.ReplaceAll(name, " ", ".")
			name = strings.ReplaceAll(name, "-", "_")

			// get time and version from context
			time := cmd.Context().Value("startTime").(time.Time)
			//


            trackCtx, cancel := context.WithTimeout(
                context.WithoutCancel(cmd.Context()), 300*time.Millisecond,
            )
            defer cancel()

            tracking.Track(trackCtx, tracking.EventCommand, map[string]string{
                tracking.TagCommandName: name,
                tracking.TagResult:      tracking.ResultSuccess,
                tracking.TagDurationMS:  strconv.FormatInt(time.Since(start).Milliseconds(), 10),
                tracking.TagCLIVersion:  version,
                tracking.TagOS:          runtime.GOOS,
            })
			
			// convert command name to tracking format

			// send command telemetry event

			// handle project.ErrEnvironmentDown

			// log errors and exit with appropriate code

			// print update notification

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