package ssh

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/shopware/shopware-cli/internal/deployment"
	shopinstall "github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/tui"
)

// prepareInitializationConfig validates scripted input without logging secret values.
func prepareInitializationConfig(input deployment.InitializationConfig) (deployment.InitializationConfig, error) {
	config := deployment.InitializationConfig{RuntimeValues: maps.Clone(input.RuntimeValues), InstallValues: maps.Clone(input.InstallValues)}
	if config.RuntimeValues == nil {
		config.RuntimeValues = make(map[string]string)
	}
	required := tui.CombineValidators(tui.ValidateRequired, tui.ValidateSingleLine)
	applicationURL := tui.CombineValidators(required, tui.ValidateHTTPURL)
	runtimeValidators := map[string]func(string) error{
		"APP_URL": applicationURL, "DATABASE_URL": validateDatabaseURL,
		"APP_SECRET": required, "APP_ENV": required, "APP_DEBUG": required,
	}
	for _, key := range []string{"APP_URL", "DATABASE_URL"} {
		if runtimeValidators[key](config.RuntimeValues[key]) != nil {
			return config, fmt.Errorf("cannot initialize deployment: invalid or missing %s", key)
		}
	}
	for key, value := range config.RuntimeValues {
		validate, ok := runtimeValidators[key]
		if !ok || validate(value) != nil {
			return config, errors.New("cannot initialize deployment: invalid runtime configuration key or value")
		}
	}
	if _, ok := config.RuntimeValues["APP_ENV"]; !ok {
		config.RuntimeValues["APP_ENV"] = "prod"
	}
	if _, ok := config.RuntimeValues["APP_DEBUG"]; !ok {
		config.RuntimeValues["APP_DEBUG"] = "0"
	}
	if len(config.InstallValues) != 0 {
		for key, value := range map[string]string{"INSTALL_LOCALE": "en-GB", "INSTALL_CURRENCY": "EUR", "INSTALL_ADMIN_USERNAME": "admin"} {
			if _, ok := config.InstallValues[key]; !ok {
				config.InstallValues[key] = value
			}
		}
		validators := map[string]func(string) error{
			"INSTALL_LOCALE": required, "INSTALL_CURRENCY": required, "INSTALL_ADMIN_USERNAME": required,
			"INSTALL_ADMIN_PASSWORD": tui.CombineValidators(required, shopinstall.ValidateAdminPassword),
			"INSTALL_ADMIN_EMAIL":    tui.OptionalValidator(tui.ValidateEmail),
			"SALES_CHANNEL_URL":      tui.OptionalValidator(applicationURL),
		}
		for key, value := range config.InstallValues {
			validate, ok := validators[key]
			if !ok || validate(value) != nil {
				return config, errors.New("cannot initialize deployment: invalid first-install configuration key or value")
			}
		}
		if validators["INSTALL_ADMIN_PASSWORD"](config.InstallValues["INSTALL_ADMIN_PASSWORD"]) != nil {
			return config, errors.New("cannot initialize deployment: invalid or missing INSTALL_ADMIN_PASSWORD")
		}
		config.InstallValues["INSTALL_CURRENCY"] = strings.ToUpper(config.InstallValues["INSTALL_CURRENCY"])
	}
	return config, nil
}
