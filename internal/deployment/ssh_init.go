package deployment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	shopinstall "github.com/shopware/shopware-cli/internal/shop/install"
)

var _ Initializer = (*SSH)(nil)

func (s *SSH) InitializeDeployment(ctx context.Context) error {
	sharedFiles, err := s.sharedFiles()
	if err != nil {
		return err
	}
	if !slices.Contains(sharedFiles, ".env.local") {
		return errors.New(`SSH deployment initialization requires ".env.local" in ssh.shared.files`)
	}
	state, err := s.inspectDeploymentInitialization(ctx)
	if err != nil {
		return err
	}
	config, confirmed, err := s.promptDeploymentInitialization(ctx, state)
	if err != nil {
		return err
	}
	if !confirmed || len(config.RuntimeValues) == 0 && len(config.InstallValues) == 0 {
		return nil
	}
	return s.applyDeploymentInitialization(ctx, config)
}

func (s *SSH) promptDeploymentInitialization(ctx context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
	updateRuntime := !state.HasRuntimeConfig
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Initialize SSH deployment").
				Description(status),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Update the existing shared runtime configuration?").
				Description("Existing unrelated variables and comments will be preserved.").
				Value(&updateRuntime),
		).WithHideFunc(func() bool { return !state.HasRuntimeConfig }),
		huh.NewGroup(
			huh.NewInput().
				Title("Application URL").
				Placeholder("https://shop.example.com").
				Validate(validateApplicationURL).
				Value(&appURL),
			huh.NewSelect[string]().
				Title("Database configuration").
				Options(
					huh.NewOption("Enter database credentials", "guided"),
					huh.NewOption("Enter a complete DATABASE_URL", "url"),
				).
				Value(&databaseMode),
		).WithHideFunc(func() bool { return !updateRuntime }),
		huh.NewGroup(
			huh.NewInput().
				Title("Database host").
				Validate(requiredDeploymentValue).
				Value(&databaseHost),
			huh.NewInput().
				Title("Database port").
				Validate(validateDatabasePort).
				Value(&databasePort),
			huh.NewInput().
				Title("Database name").
				Validate(requiredDeploymentValue).
				Value(&databaseName),
			huh.NewInput().
				Title("Database user").
				Validate(requiredDeploymentValue).
				Value(&databaseUser),
			huh.NewInput().
				Title("Database password").
				EchoMode(huh.EchoModePassword).
				Value(&databasePassword),
		).WithHideFunc(func() bool { return !updateRuntime || databaseMode != "guided" }),
		huh.NewGroup(
			huh.NewInput().
				Title("DATABASE_URL").
				Placeholder("mysql://user:password@host:3306/database").
				Validate(validateDatabaseURL).
				Value(&databaseURL),
		).WithHideFunc(func() bool { return !updateRuntime || databaseMode != "url" }),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Configure values for a possible first Shopware installation?").
				Description("Deployment Helper uses these only when it detects an uninstalled database.").
				Value(&configureInstall),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Installation locale").
				Validate(requiredDeploymentValue).
				Value(&locale),
			huh.NewInput().
				Title("Installation currency").
				Validate(requiredDeploymentValue).
				Value(&currency),
			huh.NewInput().
				Title("Administrator username").
				Validate(requiredDeploymentValue).
				Value(&adminUsername),
			huh.NewInput().
				Title("Administrator email (optional)").
				Validate(validateOptionalEmail).
				Value(&adminEmail),
			huh.NewInput().
				Title("Administrator password").
				Description("Removed from the server after a successful Deployment Helper run.").
				EchoMode(huh.EchoModePassword).
				Validate(validateAdminPassword).
				Value(&adminPassword),
			huh.NewInput().
				Title("Sales channel URL (optional)").
				Description("Leave empty to use the application URL.").
				Validate(validateOptionalApplicationURL).
				Value(&salesChannelURL),
		).WithHideFunc(func() bool { return !configureInstall }),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Write this deployment configuration to the SSH host?").
				Affirmative("Write configuration").
				Negative("Cancel").
				Value(&confirmed),
		),
	)
	if err := form.RunWithContext(ctx); err != nil {
		return sshDeploymentInitConfig{}, false, err
	}

	config := sshDeploymentInitConfig{}
	if updateRuntime {
		if databaseMode == "guided" {
			databaseURL = buildDatabaseURL(databaseHost, databasePort, databaseName, databaseUser, databasePassword)
		}
		config.RuntimeValues = map[string]string{
			"APP_ENV":      "prod",
			"APP_DEBUG":    "0",
			"APP_URL":      appURL,
			"DATABASE_URL": databaseURL,
		}
		if !slices.Contains(state.RuntimeKeys, "APP_SECRET") {
			secret, err := deploymentSecret()
			if err != nil {
				return sshDeploymentInitConfig{}, false, err
			}
			config.RuntimeValues["APP_SECRET"] = secret
		}
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

func requiredDeploymentValue(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("this cannot be empty")
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("value must be a single line")
	}
	return nil
}

func validateApplicationURL(value string) error {
	if err := requiredDeploymentValue(value); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("enter an absolute http or https URL without credentials")
	}
	return nil
}

func validateOptionalApplicationURL(value string) error {
	if value == "" {
		return nil
	}
	return validateApplicationURL(value)
}

func validateAdminPassword(value string) error {
	if err := requiredDeploymentValue(value); err != nil {
		return err
	}
	return shopinstall.ValidateAdminPassword(value)
}

func validateOptionalEmail(value string) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("email must be a single line")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return errors.New("enter a valid email address")
	}
	return nil
}

func validateDatabasePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("enter a valid TCP port")
	}
	return nil
}

func validateDatabaseURL(value string) error {
	if err := requiredDeploymentValue(value); err != nil {
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
