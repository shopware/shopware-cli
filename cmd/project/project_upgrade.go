package project

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/upgradetui"
)

var projectUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Shopware to a newer version",
	Long: "Guides you through a local Shopware upgrade: readiness checks, version selection, extension compatibility, and the guided execution.\n" +
		"In a terminal this runs as an interactive wizard. With --no-interaction (or without a terminal, e.g. CI) the upgrade runs headless:\n" +
		"--target is required there, --dry-run stops after the read-only preflight, and --no-audit continues when dependencies are blocked by security advisories.",
	RunE: func(cmd *cobra.Command, args []string) error {
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

		if !system.IsInteractionEnabled(cmd.Context()) {
			target, _ := cmd.Flags().GetString("target")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			noAudit, _ := cmd.Flags().GetBool("no-audit")

			return upgrade.NewProjectUpgrader(projectRoot, exec).RunHeadless(cmd.Context(), upgrade.HeadlessOptions{
				Target:  target,
				DryRun:  dryRun,
				NoAudit: noAudit,
				Out:     os.Stdout,
			})
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
	projectUpgradeCmd.Flags().String("target", "", "version to upgrade to (required with --no-interaction; also accepts 'recommended' or 'latest-patch')")
	projectUpgradeCmd.Flags().Bool("dry-run", false, "non-interactive mode: stop after the read-only preflight without modifying the project")
	projectUpgradeCmd.Flags().Bool("no-audit", false, "continue when dependencies are blocked by known security advisories")
}
