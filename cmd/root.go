package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/cmd/account"
	"github.com/shopware/shopware-cli/cmd/extension"
	"github.com/shopware/shopware-cli/cmd/project"
	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/update"
	"github.com/shopware/shopware-cli/logging"
)

var version = "dev"
var repo = "shopware/shopware-cli"

var rootCmd = &cobra.Command{
	Use:     "shopware-cli",
	Short:   "A cli for common Shopware tasks",
	Long:    `This application contains some utilities like extension management`,
	Version: version,
}

func Execute(ctx context.Context) {
	os.Exit(run(ctx))
}

// run executes the root command and returns the process exit code. It is kept
// separate from Execute so its deferred cleanup runs before os.Exit is called.
func run(ctx context.Context) int {
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
	
	// Check for update in the background
	updateCtx, updateCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer updateCancel()
	updateMessageChan := make(chan *update.ReleaseInfo, 1)
	go func() {
		releaseInfo, err := checkForUpdate(updateCtx, repo)
		if err != nil {
			logging.FromContext(ctx).Debugf("checking for shopware cli update failed: %v", err)
		}
		updateMessageChan <- releaseInfo
	}()
	
	start := time.Now()
	err := rootCmd.ExecuteContext(ctx)

	if cmd, _, findErr := rootCmd.Find(os.Args[1:]); findErr == nil && cmd != rootCmd && cmd.RunE != nil {
		result := tracking.ResultSuccess
		if err != nil {
			if errors.Is(err, context.Canceled) {
				result = tracking.ResultCancelled
			} else {
				result = tracking.ResultFailure
			}
		}
		name := strings.TrimPrefix(cmd.CommandPath(), "shopware-cli ")
		name = strings.ReplaceAll(name, " ", ".")
		name = strings.ReplaceAll(name, "-", "_")
		trackCtx, trackCancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
		defer trackCancel()
		tracking.Track(trackCtx, tracking.EventCommand, map[string]string{
			tracking.TagCommandName: name,
			tracking.TagResult:      result,
			tracking.TagDurationMS:  strconv.FormatInt(time.Since(start).Milliseconds(), 10),
			tracking.TagCLIVersion:  version,
			tracking.TagOS:          runtime.GOOS,
			tracking.TagIsTUI:       strconv.FormatBool(system.IsInteractionEnabled(ctx)),
		})
	}

	if errors.Is(err, project.ErrEnvironmentDown) {
		// The command already printed a human-readable status; exit 1 without
		// logging an error.
		return 1
	}

	if err != nil {
		logging.FromContext(ctx).Errorln(err)
		return 1
	}

	// todo: do not notify homebrew users if the release was recent so they can update via brew update
	// todo: hint on mac / linux to update via brew / apt (if installed via package manager?)
	// todo: distinguish if tui or non-tui command was run and print the update message accordingly
	// Wait for the update check to finish and print a message if an update is available
	newRelease := <-updateMessageChan
	if newRelease != nil {
		// Print update message to stdout
		updateMsg := fmt.Sprintf("⁺₊⋆ Update available! %s → %s ⋆₊⁺", version, newRelease.Version)
		renderedUpdateMsg := lipgloss.NewStyle().Bold(true).Foreground(tui.WarnColor).Render(updateMsg)
		fmt.Fprintln(os.Stderr, renderedUpdateMsg)
		
		// Render the tui header with the update message
		// tui.RenderHeaderWithUpdateMessage()
	}

	return 0
}

func mapAliasArgs(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}

	args := argv[1:]
	if !isSwxAlias(argv[0]) {
		return args
	}

	if len(args) > 0 {
		// Let users generate completion scripts for `swx` itself.
		if args[0] == "completion" {
			return args
		}

		// Cobra shell completion calls these internal commands.
		// Prefixing `project console` preserves swx-as-console behavior for completions.
		if args[0] == "__complete" || args[0] == "__completeNoDesc" {
			aliasedCompletionArgs := make([]string, 0, len(args)+2)
			aliasedCompletionArgs = append(aliasedCompletionArgs, args[0], "project", "console")
			aliasedCompletionArgs = append(aliasedCompletionArgs, args[1:]...)

			return aliasedCompletionArgs
		}
	}

	// When invoked via the `swx` symlink, forward everything to `project console`.
	aliasedArgs := make([]string, 0, len(args)+3)
	aliasedArgs = append(aliasedArgs, "project", "console")

	if len(args) == 0 {
		aliasedArgs = append(aliasedArgs, "list")
	} else {
		aliasedArgs = append(aliasedArgs, args...)
	}

	return aliasedArgs
}

func isSwxAlias(binaryPath string) bool {
	return strings.EqualFold(commandNameFromBinaryPath(binaryPath), "swx")
}

func commandNameFromArgs(argv []string) string {
	if len(argv) == 0 {
		return rootCmd.Use
	}

	return commandNameFromBinaryPath(argv[0])
}

func commandNameFromBinaryPath(binaryPath string) string {
	normalizedPath := strings.ReplaceAll(binaryPath, "\\", "/")
	binaryName := strings.TrimSuffix(path.Base(normalizedPath), path.Ext(normalizedPath))
	if binaryName == "" {
		return rootCmd.Use
	}

	return binaryName
}

// checkForUpdate returns the latest release info if an update is available.
func checkForUpdate(ctx context.Context, repo string) (*update.ReleaseInfo, error) {
	if !update.ShouldCheckForUpdate(version) {
		return nil, nil
	}

	return update.CheckForUpdate(ctx, repo, version)
}

func init() {
	rootCmd.SilenceErrors = true

	cobra.OnFinalize(func() {
		_ = system.CloseCaches()
	})

	rootCmd.PersistentFlags().Bool("verbose", false, "show debug output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "do not ask any interactive questions")

	project.Register(rootCmd)
	extension.Register(rootCmd)
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
