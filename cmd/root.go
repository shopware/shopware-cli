package cmd

import (
	"context"
	"os"
	"path"
	"slices"
	"strings"

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
	rootCmd.Use = commandNameFromArgs(os.Args)
	args := mapAliasArgs(os.Args)
	ctx = logging.WithLogger(ctx, logging.NewLogger(slices.Contains(args, "--verbose")))
	ctx = system.WithInteraction(ctx, !slices.Contains(args, "--no-interaction") && !slices.Contains(args, "-n") && isatty.IsTerminal(os.Stdin.Fd()))
	accountApi.SetUserAgent("shopware-cli/" + version)
	rootCmd.SetArgs(args)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logging.FromContext(ctx).Fatalln(err)
	}
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
	aliasedArgs := make([]string, 0, len(args)+2)
	aliasedArgs = append(aliasedArgs, "project", "console")
	aliasedArgs = append(aliasedArgs, args...)

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
		client, err := accountApi.NewApi(rootCmd.Context(), conf)
		if err != nil {
			return nil, err
		}
		return &account.ServiceContainer{
			Conf:          conf,
			AccountClient: client,
		}, nil
	})
}
