package ssh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/deployment"
	shopinstall "github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var _ deployment.Initializer = (*SSH)(nil)

// InitializeDeployment initializes the SSH target from a form or supplied configuration.
func (s *SSH) InitializeDeployment(ctx context.Context, options deployment.InitializationOptions) error {
	prompt, err := s.initializationPrompt(ctx, options)
	if err != nil {
		return err
	}
	return s.initializeDeployment(ctx, prompt)
}

func (s *SSH) initializationPrompt(ctx context.Context, options deployment.InitializationOptions) (func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error), error) {
	if options.Config == nil {
		if !system.IsInteractionEnabled(ctx) {
			return nil, errors.New("cannot initialize deployment without interaction: pass --init-config with a JSON file or '-' for stdin")
		}
		return s.promptDeploymentInitialization, nil
	}
	config, err := prepareInitializationConfig(*options.Config)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		if _, provided := config.RuntimeValues["APP_SECRET"]; !provided && !slices.Contains(state.RuntimeKeys, "APP_SECRET") {
			secret, err := deploymentSecret()
			if err != nil {
				return config, false, err
			}
			config.RuntimeValues["APP_SECRET"] = secret
		}
		return config, true, nil
	}, nil
}

// initializeDeployment inspects the target and writes configuration only after confirmation.
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
		return deployment.ErrInitializationCanceled
	}
	return s.initStore.Apply(ctx, config)
}

// promptDeploymentInitialization collects runtime and optional first-install values without writing them.
func (s *SSH) promptDeploymentInitialization(ctx context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
	configureInstall := tui.No
	if !state.HasCurrent && !state.HasInstallConfig {
		configureInstall = tui.Yes
	}
	appURL := ""
	if s.env != nil {
		appURL = s.env.URL
	}
	databaseHost := "127.0.0.1"
	databasePort := "3306"
	databaseName := "shopware"
	databaseUser := "shopware"
	databasePassword := ""
	locale := shopinstall.DefaultLocale
	currency := shopinstall.DefaultCurrency
	adminUsername := shopinstall.DefaultAdminUsername
	adminPassword := ""

	proceed := true
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[bool]().
			Title("Set up this environment for deployments?").
			Description("Requirements:\n• The server runs PHP 8.4 or higher\n• The SSH user is the same user that runs PHP (web server / PHP-FPM)").
			Options(huh.NewOption("Continue", true), huh.NewOption("Cancel", false)).
			Value(&proceed),
	)).WithTheme(tui.SetupTheme()).RunWithContext(ctx)
	if errors.Is(err, huh.ErrUserAborted) {
		return sshDeploymentInitConfig{}, false, nil
	}
	if err != nil || !proceed {
		return sshDeploymentInitConfig{}, false, err
	}

	requiredValue := tui.CombineValidators(tui.ValidateRequired, tui.ValidateSingleLine)
	applicationURL := tui.CombineValidators(requiredValue, tui.ValidateHTTPURL)

	localeOptions := make([]huh.Option[string], len(shopinstall.Languages))
	for i, language := range shopinstall.Languages {
		localeOptions[i] = huh.NewOption(language.Label+" ("+language.ID+")", language.ID)
	}

	for {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Application URL").
					Placeholder("https://shop.example.com").
					Validate(applicationURL).
					Value(&appURL),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Database Host").
					Validate(requiredValue).
					Value(&databaseHost),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Database Port").
					Validate(tui.ValidatePort).
					Value(&databasePort),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Database Name").
					Validate(requiredValue).
					Value(&databaseName),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Database User").
					Validate(requiredValue).
					Value(&databaseUser),
			),
			huh.NewGroup(
				huh.NewInput().
					Title("Database Password").
					EchoMode(huh.EchoModePassword).
					Value(&databasePassword),
			),
			huh.NewGroup(
				tui.NewYesNo().
					Title("Configure a first Shopware installation?").
					Description("These settings are used only when the database has not been installed yet.").
					Value(&configureInstall),
			),
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Installation Locale").
					Description("Select the primary language for your storefront").
					Height(10).
					Options(localeOptions...).
					Value(&locale),
			).WithHideFunc(func() bool { return configureInstall != tui.Yes }),
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Installation Currency").
					Description("Select the default currency for pricing").
					Options(huh.NewOptions(shopinstall.Currencies...)...).
					Value(&currency),
			).WithHideFunc(func() bool { return configureInstall != tui.Yes }),
			huh.NewGroup(
				huh.NewInput().
					Title("Administrator Username").
					Validate(requiredValue).
					Value(&adminUsername),
			).WithHideFunc(func() bool { return configureInstall != tui.Yes }),
			huh.NewGroup(
				huh.NewInput().
					Title("Administrator Password").
					Description("Removed from the server after a successful Deployment Helper run.").
					EchoMode(huh.EchoModePassword).
					Validate(tui.CombineValidators(requiredValue, shopinstall.ValidateAdminPassword)).
					Value(&adminPassword),
			).WithHideFunc(func() bool { return configureInstall != tui.Yes }),
		).WithTheme(tui.SetupTheme())
		if err := form.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return sshDeploymentInitConfig{}, false, nil
			}
			return sshDeploymentInitConfig{}, false, err
		}

		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, tui.SectionHeadingStyle.Render("Summary"))
		fmt.Fprintln(os.Stderr)
		labelStyle := lipgloss.NewStyle().Width(22)
		fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Application URL:"), appURL)
		fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Database host:"), net.JoinHostPort(databaseHost, databasePort))
		fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Database name:"), databaseName)
		fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Database user:"), databaseUser)
		if configureInstall == tui.Yes {
			fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Installation locale:"), locale)
			fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Installation currency:"), currency)
			fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("Administrator:"), adminUsername)
		} else {
			fmt.Fprintf(os.Stderr, "  %s %s\n", labelStyle.Render("First installation:"), tui.RedText.Render("No"))
		}
		fmt.Fprintln(os.Stderr)

		selectConfirm := "proceed"
		confirmForm := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Description("Write this configuration to the SSH host. For later changes, edit .env.local on the server.").
				Options(
					huh.NewOption("Proceed", "proceed"),
					huh.NewOption("Restart form", "restart"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&selectConfirm),
		)).WithTheme(tui.SetupTheme())
		if err := confirmForm.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return sshDeploymentInitConfig{}, false, nil
			}
			return sshDeploymentInitConfig{}, false, err
		}
		if selectConfirm == "cancel" {
			return sshDeploymentInitConfig{}, false, nil
		}
		if selectConfirm == "proceed" {
			break
		}
	}

	config := sshDeploymentInitConfig{
		RuntimeValues: map[string]string{
			"APP_ENV":      "prod",
			"APP_DEBUG":    "0",
			"APP_URL":      appURL,
			"DATABASE_URL": buildDatabaseURL(databaseHost, databasePort, databaseName, databaseUser, databasePassword),
		},
	}
	if !slices.Contains(state.RuntimeKeys, "APP_SECRET") {
		secret, err := deploymentSecret()
		if err != nil {
			return sshDeploymentInitConfig{}, false, err
		}
		config.RuntimeValues["APP_SECRET"] = secret
	}
	if configureInstall == tui.Yes {
		config.InstallValues = map[string]string{
			"INSTALL_LOCALE":         locale,
			"INSTALL_CURRENCY":       strings.ToUpper(currency),
			"INSTALL_ADMIN_USERNAME": adminUsername,
			"INSTALL_ADMIN_PASSWORD": adminPassword,
		}
	}
	return config, true, nil
}

// validateDatabaseURL requires a single-line MySQL or MariaDB URL with a host and database.
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

// buildDatabaseURL encodes credentials and joins the database host and port into a MySQL URL.
func buildDatabaseURL(host, port, database, username, password string) string {
	return (&url.URL{
		Scheme: "mysql",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}).String()
}

// deploymentSecret generates a random 256-bit APP_SECRET encoded as hexadecimal.
func deploymentSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate APP_SECRET: %w", err)
	}
	return hex.EncodeToString(value), nil
}
