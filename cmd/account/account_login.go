package account

import (
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login into your Shopware Account",
	Long:  "",
	RunE: func(cmd *cobra.Command, _ []string) error {
		email := services.Conf.GetAccountEmail()
		password := services.Conf.GetAccountPassword()
		newCredentials := false

		if len(email) == 0 || len(password) == 0 {
			if !system.IsInteractionEnabled(cmd.Context()) {
				return fmt.Errorf("credentials missing and interaction is disabled")
			}

			var err error
			email, password, err = askUserForEmailAndPassword()
			if err != nil {
				return err
			}

			newCredentials = true

			if err := services.Conf.SetAccountEmail(email); err != nil {
				return err
			}
			if err := services.Conf.SetAccountPassword(password); err != nil {
				return err
			}
		} else {
			logging.FromContext(cmd.Context()).Infof("Using existing credentials. Use account:logout to logout")
		}

		_, err := accountApi.NewApi(cmd.Context(), accountApi.LoginRequest{Email: email, Password: password})
		if err != nil {
			return fmt.Errorf("login failed with error: %w", err)
		}

		if newCredentials {
			err := services.Conf.Save()
			if err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
		}

		logging.FromContext(cmd.Context()).Infof("Login successful. You can now use all account commands")

		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(loginCmd)
}

func askUserForEmailAndPassword() (string, string, error) {
	var email, password string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Email").
				Validate(emptyValidator).
				Value(&email),
			huh.NewInput().
				Title("Password").
				EchoMode(huh.EchoModePassword).
				Validate(emptyValidator).
				Value(&password),
		),
	)

	if err := form.Run(); err != nil {
		return "", "", fmt.Errorf("prompt failed %w", err)
	}

	return email, password, nil
}

func emptyValidator(s string) error {
	if len(s) == 0 {
		return fmt.Errorf("this cannot be empty")
	}

	return nil
}
