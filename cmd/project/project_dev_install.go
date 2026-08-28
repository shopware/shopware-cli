package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop/install"
)

var projectDevInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Shopware non-interactively",
	Long:  "Install Shopware without the interactive TUI: starts the development environment, runs the deployment helper and saves the admin credentials to the project config. Skips the installation when the shop is already installed.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		locale, _ := cmd.Flags().GetString("locale")
		currency, _ := cmd.Flags().GetString("currency")
		adminUsername, _ := cmd.Flags().GetString("admin-username")
		adminPassword, _ := cmd.Flags().GetString("admin-password")

		opts := install.Options{
			Locale:        locale,
			Currency:      currency,
			AdminUsername: adminUsername,
			AdminPassword: adminPassword,
		}
		opts.ApplyDefaults()
		// Fail on a bad flag value before Docker gets involved.
		if err := opts.Validate(); err != nil {
			return err
		}

		env, err := setupDevEnvironment(cmd)
		if err != nil {
			return err
		}

		env.bootstrapProxyFallback(cmd)

		if err := runStep(cmd.Context(), "Starting development environment...", env.executor.StartEnvironment); err != nil {
			return err
		}

		return install.RunHeadless(cmd.Context(), env.executor, env.cfg, env.envCfg, env.projectRoot, install.HeadlessOptions{
			Install: opts,
			Out:     cmd.OutOrStdout(),
		})
	},
}

func init() {
	projectDevCmd.AddCommand(projectDevInstallCmd)

	flags := projectDevInstallCmd.Flags()
	flags.String("locale", install.DefaultLocale, "Default storefront language (e.g. en-GB, de-DE)")
	flags.String("currency", install.DefaultCurrency, "Default currency (e.g. EUR, USD)")
	flags.String("admin-username", install.DefaultAdminUsername, "Admin account username")
	flags.String("admin-password", install.DefaultAdminPassword, "Admin account password (at least 8 characters)")

	_ = projectDevInstallCmd.RegisterFlagCompletionFunc("locale", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return install.LocaleIDs(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = projectDevInstallCmd.RegisterFlagCompletionFunc("currency", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return install.Currencies, cobra.ShellCompDirectiveNoFileComp
	})
}
