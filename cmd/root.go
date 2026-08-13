package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

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

// version is replaced during release builds. During local development the
// CLI reports itself as "dev".
var version = "dev"

// rootCmd is the top-level Cobra command. All command groups, such as
// "project", "extension", and "account", are registered below it.
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
	// The executable can be called by different names. In particular, the
	// "swx" alias changes the command arguments and needs to be reflected in
	// Cobra's displayed command name as well.
	rootCmd.Use = commandNameFromArgs(os.Args)
	args := mapAliasArgs(os.Args)

	// Turn operating-system signals such as Ctrl+C into context cancellation.
	// Commands receive this context and can stop their work cleanly.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// These options affect application setup, so they are read before Cobra
	// executes the selected subcommand.
	verbose := slices.Contains(args, "--verbose")
	ctx = logging.WithLogger(ctx, logging.NewLogger(verbose))
	ctx = logging.WithVerbose(ctx, verbose)
	ctx = system.WithInteraction(ctx, !slices.Contains(args, "--no-interaction") && !slices.Contains(args, "-n") && isatty.IsTerminal(os.Stdin.Fd()))
	tui.AppVersion = version
	accountApi.SetUserAgent("shopware-cli/" + version)
	rootCmd.SetArgs(args)

	// Check for updates in the background so a network request does not delay
	// the start of the user's command. The short timeout keeps this optional
	// feature from making the CLI feel slow.
	updateCtx, updateCancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer updateCancel()
	updateChan := make(chan *update.ReleaseInfo, 1)

	go func() {
		releaseInfo, err := checkForUpdate(updateCtx, args)
		if err != nil && !errors.Is(err, update.ErrNoUpdateAvailable) {
			logging.FromContext(ctx).Debugf("checking for shopware cli update failed: %v", err)
		}
		updateChan <- releaseInfo
	}()

	// Let Cobra parse the arguments, find the matching command, and execute it.
	// The context gives the command access to cancellation and shared settings.
	start := time.Now()
	err := rootCmd.ExecuteContext(ctx)

	// Record the result of an actual subcommand. The root command itself is not
	// tracked because it does not represent a concrete user operation.
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

	// Wait for the optional update check to finish. Notifications go to stderr
	// so they do not corrupt command output written to stdout, such as JSON.
	newRelease := <-updateChan
	if newRelease != nil {
		binaryPath, err := os.Executable()
		if err != nil {
			logging.FromContext(ctx).Debugf("could not determine binary path: %v", err)
		}
		if shouldNotify(newRelease, binaryPath) {
			fmt.Fprintln(os.Stderr, update.RenderUpdateNotification(newRelease.Version, version))
		}
	}

	// This error already has a human-readable message from the project command,
	// so avoid printing a second generic error here.
	if errors.Is(err, project.ErrEnvironmentDown) {
		return 1
	}

	// All other command errors are logged once at the application boundary.
	if err != nil {
		logging.FromContext(ctx).Errorln(err)
		return 1
	}

	return 0
}

// mapAliasArgs converts the short "swx" invocation into the equivalent
// "shopware-cli project console" command. It also keeps shell-completion
// requests working when they are made through the alias.
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

		// Cobra shell completion calls these internal commands. Prefixing
		// "project console" preserves swx-as-console behavior for completions.
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

// isSwxAlias reports whether the executable was invoked as "swx".
func isSwxAlias(binaryPath string) bool {
	return strings.EqualFold(commandNameFromBinaryPath(binaryPath), "swx")
}

// commandNameFromArgs gets the executable name from os.Args. Cobra uses this
// name when displaying help and usage text.
func commandNameFromArgs(argv []string) string {
	if len(argv) == 0 {
		return rootCmd.Use
	}

	return commandNameFromBinaryPath(argv[0])
}

// commandNameFromBinaryPath extracts a binary name from Unix or Windows paths,
// and removes a possible file extension such as ".exe".
func commandNameFromBinaryPath(binaryPath string) string {
	normalizedPath := strings.ReplaceAll(binaryPath, "\\", "/")
	binaryName := strings.TrimSuffix(path.Base(normalizedPath), path.Ext(normalizedPath))
	if binaryName == "" {
		return rootCmd.Use
	}

	return binaryName
}

// checkForUpdate performs the update check only when the update package says
// that the current version and arguments allow it.
// It returns the latest release info if an update is available.
func checkForUpdate(ctx context.Context, args []string) (*update.ReleaseInfo, error) {
	if !update.ShouldCheckForUpdate(version, args) {
		return nil, update.ErrNoUpdateAvailable
	}
	return update.CheckForUpdate(ctx, version, &http.Client{Timeout: 5 * time.Second})
}

// shouldNotify returns false for Homebrew users if the new version is not yet
// available in Homebrew.
func shouldNotify(release *update.ReleaseInfo, binaryPath string) bool {
	if isUnderHomebrew(binaryPath) && release.IsRecent() {
		return false
	}
	return true
}

// isUnderHomebrew checks whether the current CLI binary is located in
// Homebrew's bin directory. This prevents premature update hints for users
// whose package manager has not published the new version yet.
func isUnderHomebrew(ghBinary string) bool {
	brewExe, err := lookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.CommandContext(context.Background(), brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(ghBinary, brewBinPrefix)
}

// lookPath wraps exec.LookPath and treats Go's ErrDot result as usable. ErrDot
// means the executable was found in the current directory.
func lookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if errors.Is(err, exec.ErrDot) {
		return path, nil
	}
	return path, err
}

// init runs automatically before main. It configures global Cobra behavior,
// defines flags shared by all commands, and builds the command tree.
func init() {
	// run handles errors itself so Cobra does not print duplicate messages.
	rootCmd.SilenceErrors = true

	// Close shared caches after Cobra has finished executing the command.
	cobra.OnFinalize(func() {
		_ = system.CloseCaches()
	})

	// Persistent flags are available on the root command and every subcommand.
	rootCmd.PersistentFlags().Bool("verbose", false, "show debug output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "do not ask any interactive questions")
	rootCmd.PersistentFlags().Bool("no-update-hint", false, "do not show update notifications")

	// Register the main command groups. Each package adds its own subcommands
	// beneath the group it owns.
	project.Register(rootCmd)
	extension.Register(rootCmd)

	// Account commands receive their services lazily. Login and logout do not
	// need an authenticated API client; other account commands do.
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
