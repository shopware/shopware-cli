// Package install holds the Shopware installation logic shared by the
// interactive dev TUI (internal/tui/dev) and the non-interactive
// `project dev install` command. The actual installation is delegated to
// vendor/bin/shopware-deployment-helper, parameterized via INSTALL_* env vars.
package install

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// Defaults applied when an Options field is left empty. They match the
// pre-selected values of the TUI install wizard.
const (
	DefaultLocale        = "en-GB"
	DefaultCurrency      = "EUR"
	DefaultAdminUsername = "admin"
	DefaultAdminPassword = "shopware"
)

// MinAdminPasswordLength is the minimum admin password length enforced by the
// Shopware core (user:create / system:install). Validating it here rejects
// too-short passwords up front instead of failing late during the
// deployment-helper run.
const MinAdminPasswordLength = 8

// Language is a storefront locale selectable during installation.
type Language struct {
	ID    string
	Label string
}

// Languages are the locales offered by the install wizard, in display order.
// The IDs are Shopware locale codes accepted by system:install.
var Languages = []Language{
	{"en-GB", "English (UK)"},
	{"en-US", "English (US)"},
	{"de-DE", "Deutsch"},
	{"cs-CZ", "Čeština"},
	{"da-DK", "Dansk"},
	{"es-ES", "Español"},
	{"fr-FR", "Français"},
	{"it-IT", "Italiano"},
	{"nl-NL", "Nederlands"},
	{"nn-NO", "Norsk"},
	{"pl-PL", "Język polski"},
	{"pt-PT", "Português"},
	{"sv-SE", "Svenska"},
}

// Currencies are the ISO codes offered by the install wizard.
var Currencies = []string{"EUR", "USD", "GBP", "PLN", "CHF", "SEK", "DKK", "NOK", "CZK"}

// LocaleIDs returns the locale codes of Languages, for flag completion and
// validation errors.
func LocaleIDs() []string {
	ids := make([]string, len(Languages))
	for i, lang := range Languages {
		ids[i] = lang.ID
	}
	return ids
}

// ValidateLocale checks the locale against the Languages allow-list.
func ValidateLocale(locale string) error {
	for _, lang := range Languages {
		if lang.ID == locale {
			return nil
		}
	}
	return fmt.Errorf("unknown locale %q, valid values: %s", locale, strings.Join(LocaleIDs(), ", "))
}

// ValidateCurrency checks the currency against the Currencies allow-list.
func ValidateCurrency(currency string) error {
	for _, c := range Currencies {
		if c == currency {
			return nil
		}
	}
	return fmt.Errorf("unknown currency %q, valid values: %s", currency, strings.Join(Currencies, ", "))
}

// ValidateAdminPassword mirrors the Shopware core password length requirement.
func ValidateAdminPassword(password string) error {
	if len([]rune(password)) < MinAdminPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", MinAdminPasswordLength)
	}
	return nil
}

// Options parameterize a Shopware installation.
type Options struct {
	Locale        string
	Currency      string
	AdminUsername string
	AdminPassword string
}

// ApplyDefaults fills empty fields with the wizard defaults.
func (o *Options) ApplyDefaults() {
	if o.Locale == "" {
		o.Locale = DefaultLocale
	}
	if o.Currency == "" {
		o.Currency = DefaultCurrency
	}
	if o.AdminUsername == "" {
		o.AdminUsername = DefaultAdminUsername
	}
	if o.AdminPassword == "" {
		o.AdminPassword = DefaultAdminPassword
	}
}

// Validate checks locale, currency and password against the same rules the
// TUI wizard enforces.
func (o Options) Validate() error {
	if err := ValidateLocale(o.Locale); err != nil {
		return err
	}
	if err := ValidateCurrency(o.Currency); err != nil {
		return err
	}
	return ValidateAdminPassword(o.AdminPassword)
}

// CustomCredentials reports whether the admin credentials differ from the
// defaults. Telemetry sends only this flag, never the credentials.
func (o Options) CustomCredentials() bool {
	return o.AdminUsername != DefaultAdminUsername || o.AdminPassword != DefaultAdminPassword
}

// IsInstalled reports whether the shop is already installed.
func IsInstalled(ctx context.Context, exec executor.Executor) bool {
	return exec.ConsoleCommand(ctx, "system:is-installed").Run() == nil
}

// Run installs Shopware by running the deployment helper with the INSTALL_*
// env vars. Combined stdout/stderr is emitted line by line via onLine.
func Run(ctx context.Context, exec executor.Executor, opts Options, onLine func(string)) error {
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE":         opts.Locale,
		"INSTALL_CURRENCY":       opts.Currency,
		"INSTALL_ADMIN_USERNAME": opts.AdminUsername,
		"INSTALL_ADMIN_PASSWORD": opts.AdminPassword,
	})
	p := withEnv.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")

	lw := tui.NewLineWriter(onLine)
	err := p.RunWithOutput(lw)
	lw.Flush()
	return err
}

// PersistCredentials stores the admin credentials of a fresh installation in
// the project config so API commands work out of the box. ResolveEnvironment
// can return a copy that is not stored in cfg.Environments (the deprecated
// top-level url/admin_api form) — credentials set on that copy would be lost
// on write, so they are mirrored onto the top-level config in that case.
func PersistCredentials(cfg *shop.Config, envCfg *shop.EnvironmentConfig, projectRoot string, opts Options) error {
	adminApi := &shop.ConfigAdminApi{
		Username: opts.AdminUsername,
		Password: opts.AdminPassword,
	}
	envCfg.AdminApi = adminApi

	stored := false
	for _, env := range cfg.Environments {
		if env == envCfg {
			stored = true
			break
		}
	}
	if !stored {
		cfg.AdminApi = adminApi
	}

	return shop.WriteConfig(cfg, projectRoot)
}
