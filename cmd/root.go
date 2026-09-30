package cmd

import (
	"context"
	"errors"
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
	"github.com/shopware/shopware-cli/internal/cliversion"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

// version is the legacy ldflags target (-X 'github.com/shopware/shopware-cli/cmd.version=...').
// It is kept until all build platforms set internal/cliversion.Version directly.
var version string

var rootCmd = &cobra.Command{
	Use:   "shopware-cli",
	Short: "Build, develop, and manage Shopware projects and extensions",
	Long:  `Build, develop, and manage Shopware projects and extensions from the command line.`,
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
	rootCmd.SetArgs(args)

	updateHandle, updateCancel := startUpdateCheck(ctx, args)
	defer updateCancel()

	start := time.Now()
	err := rootCmd.ExecuteContext(ctx)

	trackCommandExecution(ctx, args, start, err)
	printUpdateHint(ctx, os.Stderr, updateHandle.Wait(ctx).Release)

	return exitCode(ctx, err)
}

// exitCode maps the command error to the process exit code. Status errors are
// already printed by the command as a human-readable status, so they exit 1
// without logging the error again.
func exitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}

	if !errors.Is(err, project.ErrEnvironmentDown) && !errors.Is(err, project.ErrProxyNotRegistered) && !errors.Is(err, project.ErrProxyVerificationFailed) {
		logging.FromContext(ctx).Errorln(err)
	}

	return 1
}

func init() {
	if version != "" {
		cliversion.Version = version
	}
	rootCmd.Version = cliversion.Version

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

	rootCmd.PersistentFlags().Bool("verbose", false, "Show debug logs and detailed tool output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "Run without prompting; commands use defaults or fail where input is needed")
	rootCmd.PersistentFlags().Bool("no-update-hint", false, "Skip checking for a newer shopware-cli version")

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
