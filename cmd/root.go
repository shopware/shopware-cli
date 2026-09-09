package cmd

import (
	"context"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/cmd/account"
	"github.com/shopware/shopware-cli/cmd/ai"
	"github.com/shopware/shopware-cli/cmd/extension"
	"github.com/shopware/shopware-cli/cmd/project"
	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "shopware-cli",
	Short:   "A cli for common Shopware tasks",
	Long:    `This application contains some utilities like extension management`,
	Version: version,
}

// Execute runs the root command and returns the process exit code after cleanup.
func Execute(ctx context.Context) int {
	rootCmd.Use = commandNameFromArgs(os.Args)
	args := mapAliasArgs(os.Args)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	verbose := slices.Contains(args, "--verbose")
	ctx = logging.WithLogger(ctx, logging.NewLogger(verbose))
	ctx = logging.WithVerbose(ctx, verbose)
	ctx = system.WithInteraction(ctx, !slices.Contains(args, "--no-interaction") && !slices.Contains(args, "-n") && isatty.IsTerminal(os.Stdin.Fd()))
	tui.AppVersion = version
	accountApi.SetUserAgent("shopware-cli/" + version)
	rootCmd.SetArgs(args)

	updateHandle, updateCancel := startUpdateCheck(ctx, args)
	defer updateCancel()

	start := time.Now()
	err := rootCmd.ExecuteContext(ctx)

	trackCommandExecution(ctx, os.Args[1:], start, err)
	printUpdateHint(ctx, updateHandle.Wait(ctx).Release)

	if err != nil {
		logging.FromContext(ctx).Errorln(err)
		return 1
	}

	return 0
}

func init() {
	rootCmd.SilenceErrors = true

	// Cobra prints the usage block for every error a command returns. Flags,
	// argument counts and flag groups are validated before the pre-run hooks
	// fire, so silencing usage here keeps it for invocation mistakes while
	// errors returned from RunE only print the error itself. Traversal is
	// enabled so subtrees with their own PersistentPreRunE (account) still run
	// this hook.
	cobra.EnableTraverseRunHooks = true
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return nil
	}

	cobra.OnFinalize(func() {
		_ = system.CloseCaches()
	})

	rootCmd.PersistentFlags().Bool("verbose", false, "Show debug output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "Do not ask any interactive questions")
	rootCmd.PersistentFlags().Bool("no-update-hint", false, "Do not show update notifications")

	project.Register(rootCmd)
	extension.Register(rootCmd)
	ai.Register(rootCmd)
	account.Register(rootCmd, func(commandName string) (*account.ServiceContainer, error) {
		if commandName == "login" || commandName == "logout" {
			return &account.ServiceContainer{
				AccountClient: nil,
			}, nil
		}
		client, err := accountApi.NewApi(rootCmd.Context())
		if err != nil {
			return nil, err
		}
		return &account.ServiceContainer{
			AccountClient: client,
		}, nil
	})
}
