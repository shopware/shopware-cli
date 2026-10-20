package deployment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"charm.land/huh/v2"

	shopinstall "github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var _ Initializer = (*SSH)(nil)

func (s *SSH) InitializeDeployment(ctx context.Context) error {
	return s.initializeDeployment(ctx, s.promptDeploymentInitialization)
}

func (s *SSH) initializeDeployment(ctx context.Context, prompt func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error)) error {
	state, err := s.initStore.Inspect(ctx)
	if err != nil {
		return err
	}
	if state.Initialized {
		logging.FromContext(ctx).Info("Deployment is already initialized")
		return nil
	}
	config, confirmed, err := prompt(ctx, state)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrInitializationCanceled
	}
	return s.initStore.Apply(ctx, config)
}

func (s *SSH) promptDeploymentInitialization(ctx context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
	configureInstall := !state.HasCurrent && !state.HasInstallConfig
	confirmed := true
	databaseMode := "guided"
	appURL := ""
	if s.env != nil {
		appURL = s.env.URL
	}
	databaseURL := ""
	databaseHost := "127.0.0.1"
	databasePort := "3306"
	databaseName := "shopware"
	databaseUser := "shopware"
	databasePassword := ""
	locale := "en-GB"
	currency := "EUR"
	adminUsername := "admin"
	adminEmail := ""
	adminPassword := ""
	salesChannelURL := ""

	status := "No active release was found."
	if state.HasCurrent {
		status = "An active release was found."
	}
	if state.HasRuntimeConfig {
		status += " Runtime configuration already exists."
	}
	if state.HasInstallConfig {
		status += " Pending first-install values already exist."
	}

	requiredValue := tui.CombineValidators(tui.ValidateRequired, tui.ValidateSingleLine)
	applicationURL := tui.CombineValidators(requiredValue, tui.ValidateHTTPURL)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Initialize SSH deployment").
				Description(status),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Application URL").
				Placeholder("https://shop.example.com").
				Validate(applicationURL).
				Value(&appURL),
			huh.NewSelect[string]().
				Title("Database configuration").
				Options(
					huh.NewOption("Enter database credentials", "guided"),
					huh.NewOption("Enter a complete DATABASE_URL", "url"),
				).
				Value(&databaseMode),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Database host").
				Validate(requiredValue).
				Value(&databaseHost),
			huh.NewInput().
				Title("Database port").
				Validate(tui.ValidatePort).
				Value(&databasePort),
			huh.NewInput().
				Title("Database name").
				Validate(requiredValue).
				Value(&databaseName),
			huh.NewInput().
				Title("Database user").
				Validate(requiredValue).
				Value(&databaseUser),
			huh.NewInput().
				Title("Database password").
				EchoMode(huh.EchoModePassword).
				Value(&databasePassword),
		).WithHideFunc(func() bool { return databaseMode != "guided" }),
		huh.NewGroup(
			huh.NewInput().
				Title("DATABASE_URL").
				Placeholder("mysql://user:password@host:3306/database").
				Validate(validateDatabaseURL).
				Value(&databaseURL),
		).WithHideFunc(func() bool { return databaseMode != "url" }),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Configure values for a possible first Shopware installation?").
				Description("Deployment Helper uses these only when it detects an uninstalled database.").
				Value(&configureInstall),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Installation locale").
				Validate(requiredValue).
				Value(&locale),
			huh.NewInput().
				Title("Installation currency").
				Validate(requiredValue).
				Value(&currency),
			huh.NewInput().
				Title("Administrator username").
				Validate(requiredValue).
				Value(&adminUsername),
			huh.NewInput().
				Title("Administrator email (optional)").
				Validate(tui.OptionalValidator(tui.ValidateEmail)).
				Value(&adminEmail),
			huh.NewInput().
				Title("Administrator password").
				Description("Removed from the server after a successful Deployment Helper run.").
				EchoMode(huh.EchoModePassword).
				Validate(tui.CombineValidators(requiredValue, shopinstall.ValidateAdminPassword)).
				Value(&adminPassword),
			huh.NewInput().
				Title("Sales channel URL (optional)").
				Description("Leave empty to use the application URL.").
				Validate(tui.OptionalValidator(applicationURL)).
				Value(&salesChannelURL),
		).WithHideFunc(func() bool { return !configureInstall }),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Write this deployment configuration to the SSH host?").
				Description("Initialization runs once. Existing unrelated variables are preserved; edit .env.local manually for later changes.").
				Affirmative("Write configuration").
				Negative("Cancel").
				Value(&confirmed),
		),
	)
	if err := form.RunWithContext(ctx); err != nil {
		return sshDeploymentInitConfig{}, false, err
	}

	if databaseMode == "guided" {
		databaseURL = buildDatabaseURL(databaseHost, databasePort, databaseName, databaseUser, databasePassword)
	}
	config := sshDeploymentInitConfig{
		RuntimeValues: map[string]string{
			"APP_ENV":      "prod",
			"APP_DEBUG":    "0",
			"APP_URL":      appURL,
			"DATABASE_URL": databaseURL,
		},
	}
	if !slices.Contains(state.RuntimeKeys, "APP_SECRET") {
		secret, err := deploymentSecret()
		if err != nil {
			return sshDeploymentInitConfig{}, false, err
		}
		config.RuntimeValues["APP_SECRET"] = secret
	}
	if configureInstall {
		config.InstallValues = map[string]string{
			"INSTALL_LOCALE":         locale,
			"INSTALL_CURRENCY":       strings.ToUpper(currency),
			"INSTALL_ADMIN_USERNAME": adminUsername,
			"INSTALL_ADMIN_PASSWORD": adminPassword,
		}
		if adminEmail != "" {
			config.InstallValues["INSTALL_ADMIN_EMAIL"] = adminEmail
		}
		if salesChannelURL != "" {
			config.InstallValues["SALES_CHANNEL_URL"] = salesChannelURL
		}
	}
	return config, confirmed, nil
}

func validateDatabaseURL(value string) error {
	if err := tui.CombineValidators(tui.ValidateRequired, tui.ValidateSingleLine)(value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "mysql" && parsed.Scheme != "mariadb") || parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
		return errors.New("enter a mysql or mariadb URL with host and database name")
	}
	return nil
}

func buildDatabaseURL(host, port, database, username, password string) string {
	return (&url.URL{
		Scheme: "mysql",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}).String()
}

func deploymentSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate APP_SECRET: %w", err)
	}
	return hex.EncodeToString(value), nil
}
