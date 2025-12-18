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
		var content []byte
		var err error

		// Create URL input form
		urlForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Shop-URL example: http://localhost").
					Validate(emptyValidator).
					Value(&config.URL),
			),
		)

		if err := urlForm.Run(); err != nil {
			return err
		}

		if err = askApi(config); err != nil {
			return err
		}

		if content, err = yaml.Marshal(config); err != nil {
			return err
		}

		if err := os.WriteFile(".shopware-project.yml", content, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Info("Created .shopware-project.yml")

		return nil
	},
}

func askApi(config *shop.Config) error {
	var configureApi bool
	var authType string

	// Ask if user wants to configure API access
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Configure admin-api access").
				Value(&configureApi),
		),
	)

	if err := confirmForm.Run(); err != nil {
		return err
	}

	if !configureApi {
		return nil
	}

	// Choose auth type
	authTypeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Auth type").
				Options(
					huh.NewOption("user-password", "user-password"),
					huh.NewOption("integration", "integration"),
				).
				Value(&authType),
		),
	)

	if err := authTypeForm.Run(); err != nil {
		return err
	}

	apiConfig := shop.ConfigAdminApi{}
	config.AdminApi = &apiConfig

	if authType == "integration" {
		var clientId, clientSecret string

		// Integration auth form
		integrationForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Client-ID").
					Validate(emptyValidator).
					Value(&clientId),
				huh.NewInput().
					Title("Client-Secret").
					Validate(emptyValidator).
					Value(&clientSecret),
			),
		)

		if err := integrationForm.Run(); err != nil {
			return err
		}

		apiConfig.ClientId = clientId
		apiConfig.ClientSecret = clientSecret

		return nil
	}

	var username, password string

	// User-password auth form
	userPasswordForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Admin User").
				Validate(emptyValidator).
				Value(&username),
			huh.NewInput().
				Title("Admin Password").
				Validate(emptyValidator).
				Value(&password),
		),
	)

	if err := userPasswordForm.Run(); err != nil {
		return err
	}

	apiConfig.Username = username
	apiConfig.Password = password

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
