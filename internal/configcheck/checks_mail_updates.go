package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// DisabledMailUpdatesCheck flags shopware.mail.update_mail_variables_on_send = true.
// Updating mail variables when sending each mail causes extra DB roundtrips;
// disabling it is a documented performance tweak.
type DisabledMailUpdatesCheck struct{}

func (DisabledMailUpdatesCheck) ID() string { return "mail-update-variables-on-send" }

func (DisabledMailUpdatesCheck) Run(cfg *symfonyconfig.Config) []Result {
	const path = "shopware.mail.update_mail_variables_on_send"
	v, ok := cfg.GetBool(path)
	if !ok || !v {
		return nil
	}
	return []Result{{
		ID:       "mail-update-variables-on-send",
		Title:    "Mail variables are refreshed on every send",
		Message:  "shopware.mail.update_mail_variables_on_send is true. Set it to false to skip the per-send variable refresh.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
		Path:     path,
	}}
}
