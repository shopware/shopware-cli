package project

import (
	"errors"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/compatibility"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var projectConfigInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new project config",
	Long: `Create a new .config/shopware-project.yml config in the current directory.

Shop URL and Admin API credentials are written under environments.local.
Omit -e / --env on later commands to target that environment.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("this command requires interaction, but interaction is disabled")
		}

		force, _ := cmd.Flags().GetBool("force")

		// first check if a config already exists
		actualProjectConfigPath := shop.SearchConfigPath(cmd.Context(), ".", projectConfigPath)
		if _, err := os.Stat(actualProjectConfigPath); err == nil && !force {
			return fmt.Errorf("%s already exists (pass --force to overwrite)", actualProjectConfigPath)
		}

		config := &shop.Config{
			CompatibilityDate: compatibility.DefaultDate(),
		}

		if err := askProjectConfig(config); err != nil {
			return err
		}

		// write to the resolved file, so --force overwrites a legacy config in place
		config.SetStorageLocation(actualProjectConfigPath)

		if err := shop.WriteConfig(config, "."); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Created %s", config.GetStorageLocation())

		return nil
	},
}

func askProjectConfig(config *shop.Config) error {
	var configureApi bool
	var authType string
	var clientId, clientSecret string
	var username, password string
	var shopURL string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Shop-URL example: http://localhost").
				Validate(emptyValidator).
				Value(&shopURL),
			huh.NewConfirm().
				Title("Configure Admin API access").
				Value(&configureApi),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Auth type").
				Options(
					huh.NewOption("user-password", "user-password"),
					huh.NewOption("integration", "integration"),
				).
				Value(&authType),
		).WithHideFunc(func() bool { return !configureApi }),
		huh.NewGroup(
			huh.NewInput().
				Title("Client-ID").
				Validate(emptyValidator).
				Value(&clientId),
			huh.NewInput().
				Title("Client-Secret").
				Validate(emptyValidator).
				Value(&clientSecret),
		).WithHideFunc(func() bool {
			return !configureApi || authType != "integration"
		}),
		huh.NewGroup(
			huh.NewInput().
				Title("Admin User").
				Validate(emptyValidator).
				Value(&username),
			huh.NewInput().
				Title("Admin Password").
				Validate(emptyValidator).
				Value(&password),
		).WithHideFunc(func() bool {
			return !configureApi || authType != "user-password"
		}),
	)

	if err := form.Run(); err != nil {
		return err
	}

	var adminApi *shop.ConfigAdminApi
	if configureApi {
		adminApi = &shop.ConfigAdminApi{}
		if authType == "integration" {
			adminApi.ClientId = clientId
			adminApi.ClientSecret = clientSecret
		} else {
			adminApi.Username = username
			adminApi.Password = password
		}
	}

	config.SetLocalShop(shopURL, adminApi)

	return nil
}

func init() {
	projectConfigInitCmd.Flags().Bool("force", false, "Overwrite existing .config/shopware-project.yml")
	projectConfigCmd.AddCommand(projectConfigInitCmd)
}

func emptyValidator(s string) error {
	if len(s) == 0 {
		return errors.New("this cannot be empty")
	}

	return nil
}
