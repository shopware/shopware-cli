package cmd

import (
	"context"
	"os"
	"slices"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/cmd/account"
	"github.com/shopware/shopware-cli/cmd/extension"
	"github.com/shopware/shopware-cli/cmd/project"
	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/config"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var (
	cfgFile string
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "shopware-cli",
	Short:   "A cli for common Shopware tasks",
	Long:    `This application contains some utilities like extension management`,
	Version: version,
}

func Execute(ctx context.Context) {
	ctx = logging.WithLogger(ctx, logging.NewLogger(slices.Contains(os.Args, "--verbose")))
	ctx = system.WithInteraction(ctx, !slices.Contains(os.Args, "--no-interaction") && !slices.Contains(os.Args, "-n") && isatty.IsTerminal(os.Stdin.Fd()))
	accountApi.SetUserAgent("shopware-cli/" + version)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logging.FromContext(ctx).Fatalln(err)
	}
}

func init() {
	rootCmd.SilenceErrors = true

	cobra.OnInitialize(func() {
		_ = config.InitConfig(cfgFile)
	})

	cobra.OnFinalize(func() {
		_ = system.CloseCaches()
	})

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.shopware-cli.yaml)")
	rootCmd.PersistentFlags().Bool("verbose", false, "show debug output")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "do not ask any interactive questions")

	project.Register(rootCmd)
	extension.Register(rootCmd)
	account.Register(rootCmd, func(commandName string) (*account.ServiceContainer, error) {
		err := config.InitConfig(cfgFile)
		if err != nil {
			return nil, err
		}
		conf := config.Config{}
		if commandName == "login" || commandName == "logout" {
			return &account.ServiceContainer{
				Conf:          conf,
				AccountClient: nil,
			}, nil
		}
		client, err := accountApi.NewApi(rootCmd.Context(), nil)
		if err != nil {
			return nil, err
		}
		return &account.ServiceContainer{
			Conf:          conf,
			AccountClient: client,
		}, nil
	})
}
