package account

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"
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

		client, err := accountApi.NewApi(cmd.Context(), accountApi.LoginRequest{Email: email, Password: password})
		if err != nil {
			return fmt.Errorf("login failed with error: %w", err)
		}

		if companyId := services.Conf.GetAccountCompanyId(); companyId > 0 {
			err = changeAPIMembership(cmd.Context(), client, companyId)
			if err != nil {
				return fmt.Errorf("cannot change company member ship: %w", err)
			}
		}

		if newCredentials {
			err := services.Conf.Save()
			if err != nil {
				return fmt.Errorf("cannot save config: %w", err)
			}
		}

		profile, err := client.GetMyProfile(cmd.Context())
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof(
			"Hey %s %s. You are now authenticated on company %s and can use all account commands",
			profile.PersonalData.FirstName,
			profile.PersonalData.LastName,
			client.GetActiveMembership().Company.Name,
		)

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

func changeAPIMembership(ctx context.Context, client *accountApi.Client, companyID int) error {
	if companyID == 0 || client.GetActiveCompanyID() == companyID {
		logging.FromContext(ctx).Debugf("Client is on correct membership skip")
		return nil
	}

	for _, membership := range client.GetMemberships() {
		if membership.Company.Id == companyID {
			logging.FromContext(ctx).Debugf("Changing member ship from %s (%d) to %s (%d)", client.ActiveMembership.Company.Name, client.ActiveMembership.Company.Id, membership.Company.Name, membership.Company.Id)
			return client.ChangeActiveMembership(ctx, membership)
		}
	}

	return fmt.Errorf("could not find configured company with id %d", companyID)
}
