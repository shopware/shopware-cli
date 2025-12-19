package project

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var projectConfigInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Creates a new project config in current dir",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !system.IsInteractionEnabled(cmd.Context()) {
			return fmt.Errorf("this command requires interaction, but interaction is disabled")
		}

		config := &shop.Config{}

		if err := askProjectConfig(config); err != nil {
			return err
		}

		content, err := yaml.Marshal(config)
		if err != nil {
			return err
		}

		if err := os.WriteFile(".shopware-project.yml", content, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Info("Created .shopware-project.yml")

		return nil
	},
}

func askProjectConfig(config *shop.Config) error {
	var configureApi bool
	var authType string
	var clientId, clientSecret string
	var username, password string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Shop-URL example: http://localhost").
				Validate(emptyValidator).
				Value(&config.URL),
			huh.NewConfirm().
				Title("Configure admin-api access").
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

	if !configureApi {
		return nil
	}

	config.AdminApi = &shop.ConfigAdminApi{}

	if authType == "integration" {
		config.AdminApi.ClientId = clientId
		config.AdminApi.ClientSecret = clientSecret
		return nil
	}

	config.AdminApi.Username = username
	config.AdminApi.Password = password

	return nil
}

func init() {
	projectConfigCmd.AddCommand(projectConfigInitCmd)
}

func emptyValidator(s string) error {
	if len(s) == 0 {
		return fmt.Errorf("this cannot be empty")
	}

	return nil
}
