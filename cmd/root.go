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

var version = "dev"

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

	// Check for update in the background. Interactive TUIs consume the same
	// result through the shared header's update hint.
	updateCtx, updateCancel := context.WithTimeout(ctx, 900*time.Millisecond)
	defer updateCancel()
	updateHandle := update.NewCheckHandle()
	tui.UpdateAvailable = func(waitCtx context.Context) bool {
		result := updateHandle.Wait(waitCtx)
		if result.Release == nil {
			return false
		}
		binaryPath, _ := os.Executable()
		return shouldNotify(result.Release, binaryPath)
	}

	go func() {
		releaseInfo, err := checkForUpdate(updateCtx, args)
		if err != nil && !errors.Is(err, update.ErrNoUpdateAvailable) {
			logging.FromContext(ctx).Debugf("checking for shopware cli update failed: %v", err)
		}
		updateHandle.Complete(update.CheckResult{Release: releaseInfo, Err: err})
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

	updateResult := updateHandle.Wait(ctx)
	newRelease := updateResult.Release
	if newRelease != nil {
		binaryPath, err := os.Executable()
		if err != nil {
			logging.FromContext(ctx).Debugf("could not determine binary path: %v", err)
		}
		if shouldNotify(newRelease, binaryPath) && update.ShouldPrintUpdateHint() {
			fmt.Fprintln(os.Stderr, update.RenderUpdateNotification(newRelease.Version, version))
			if err := update.MarkUpdateNotificationPrinted(); err != nil {
				logging.FromContext(ctx).Debugf("could not save update notification timestamp: %v", err)
			}
		}
	}

	if errors.Is(err, project.ErrEnvironmentDown) || errors.Is(err, project.ErrProxyNotRegistered) {
		// The command already printed a human-readable status; exit 1 without
		// logging an error.
		return 1
	}

	if err != nil {
		logging.FromContext(ctx).Errorln(err)
		return 1
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
func checkForUpdate(ctx context.Context, args []string) (*update.ReleaseInfo, error) {
	if !update.ShouldCheckForUpdate(version, args) {
		return nil, update.ErrNoUpdateAvailable
	}
	return update.CheckForUpdate(ctx, version, &http.Client{Timeout: 5 * time.Second})
}

// shouldNotify returns false for Homebrew users if the new version is not yet available in Homebrew
func shouldNotify(release *update.ReleaseInfo, binaryPath string) bool {
	if isUnderHomebrew(binaryPath) && release.IsRecent() {
		return false
	}
	return true
}

// Check whether the gh binary was found under the Homebrew prefix
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

// lookPath allows safe execution of the LookPath function, handling the ErrDot case.
func lookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if errors.Is(err, exec.ErrDot) {
		return path, nil
	}
	return path, err
}

func init() {
	rootCmd.SilenceErrors = true

	cobra.OnFinalize(func() {
		_ = system.CloseCaches()
	})

	rootCmd.PersistentFlags().Bool("verbose", false, "show debug output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "do not ask any interactive questions")
	rootCmd.PersistentFlags().Bool("no-update-hint", false, "do not show update notifications")

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
