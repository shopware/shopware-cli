package cmd

import (
	"context"
	"errors"
	"os"
	"os/signal"
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
	"github.com/shopware/shopware-cli/cmd/hub"
	"github.com/shopware/shopware-cli/cmd/project"
	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
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
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	verbose := slices.Contains(os.Args, "--verbose")
	ctx = logging.WithLogger(ctx, logging.NewLogger(verbose))
	ctx = logging.WithVerbose(ctx, verbose)
	ctx = system.WithInteraction(ctx, !slices.Contains(os.Args, "--no-interaction") && !slices.Contains(os.Args, "-n") && isatty.IsTerminal(os.Stdin.Fd()))
	accountApi.SetUserAgent("shopware-cli/" + version)

	start := time.Now()
	err := rootCmd.ExecuteContext(ctx)

	if cmd, _, findErr := rootCmd.Find(os.Args[1:]); findErr == nil && cmd != rootCmd && cmd.RunE != nil {
		result := "success"
		if err != nil {
			if errors.Is(err, context.Canceled) {
				result = "cancelled"
			} else {
				result = "failure"
			}
		}
		name := strings.TrimPrefix(cmd.CommandPath(), "shopware-cli ")
		name = strings.ReplaceAll(name, " ", ".")
		name = strings.ReplaceAll(name, "-", "_")
		trackCtx, trackCancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
		defer trackCancel()
		tracking.Track(trackCtx, "command", map[string]string{
			"command_name": name,
			"result":       result,
			"duration_ms":  strconv.FormatInt(time.Since(start).Milliseconds(), 10),
			"cli_version":  version,
			"os":           runtime.GOOS,
			"is_tui":       strconv.FormatBool(system.IsInteractionEnabled(ctx)),
		})
	}

	if err != nil {
		logging.FromContext(ctx).Fatalln(err)
	}
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
	hub.Register(rootCmd, func() (*hub.ServiceContainer, error) {
		client, err := accountApi.NewApi(rootCmd.Context())
		if err != nil {
			return nil, err
		}
		return &hub.ServiceContainer{
			AccountClient: client,
		}, nil
	})
}
