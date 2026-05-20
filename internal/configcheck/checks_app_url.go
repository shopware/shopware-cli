package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// AppUrlCheckDisabledCheck flags APP_URL_CHECK_DISABLED not being set to a
// truthy value. The HTTP self-check Shopware runs on every request can be
// expensive and is not needed when the URL is known-good.
type AppUrlCheckDisabledCheck struct{}

func (AppUrlCheckDisabledCheck) ID() string { return "app-url-check-not-disabled" }

func (AppUrlCheckDisabledCheck) Run(cfg *symfonyconfig.Config) []Result {
	const name = "APP_URL_CHECK_DISABLED"
	v, _ := cfg.LookupEnv(name)
	if isTruthy(v) {
		return nil
	}
	return []Result{{
		ID:       "app-url-check-not-disabled",
		Title:    "APP_URL check is enabled",
		Message:  "APP_URL_CHECK_DISABLED is not set to true. Set APP_URL_CHECK_DISABLED=1 in .env to skip the per-request URL self-check.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
		Path:     name,
	}}
}

func isTruthy(s string) bool {
	switch s {
	case "1", "true", "yes", "on", "TRUE", "Yes", "ON":
		return true
	}
	return false
}
