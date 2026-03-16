package project

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/devtui"
	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDevCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the interactive development dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		if cfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
			return shop.ErrDevModeNotSupported
		}

		envCfg, err := cfg.ResolveEnvironment(environmentName)
		if err != nil {
			return err
		}

		exec, err := executor.New(envCfg, cfg)
		if err != nil {
			return err
		}

		if exec.Type() == "docker" {
			if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(cfg)); err != nil {
				return err
			}
		}

		m := devtui.New(devtui.Options{
			ProjectRoot: projectRoot,
			Config:      cfg,
			EnvConfig:   envCfg,
			Executor:    exec,
		})

		p := tea.NewProgram(m)
		_, err = p.Run()
		return err
	},
}

func init() {
	projectRootCmd.AddCommand(projectDevCmd)
}
