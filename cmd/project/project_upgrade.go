package project

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/upgradetui"
)

var projectUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Shopware to a newer version",
	Long:  "Interactive wizard that guides you through a local Shopware upgrade: readiness checks, version selection, extension compatibility, and the guided execution. Use `project upgrade-check` for the non-interactive compatibility check.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isatty.IsTerminal(os.Stdin.Fd()) {
			return fmt.Errorf("the upgrade wizard needs an interactive terminal; run `shopware-cli project upgrade-check` in scripts instead")
		}

		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		envCfg, err := cfg.ResolveEnvironment(environmentName)
		if err != nil {
			return err
		}

		exec, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		envName := environmentName
		if envName == "" {
			envName = envCfg.Type
		}
		if envName == "" {
			envName = "local"
		}

		shell := upgradetui.NewApp(upgradetui.Options{
			ProjectRoot: projectRoot,
			EnvName:     envName,
			Executor:    exec,
		})

		_, err = shell.Run()
		return err
	},
}

func init() {
	projectRootCmd.AddCommand(projectUpgradeCmd)
}
