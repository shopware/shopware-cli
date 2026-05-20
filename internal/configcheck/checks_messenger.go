package configcheck

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// MessengerAutoSetupCheck flags any Messenger transport DSN with
// auto_setup=true (which is also the implicit default). In production the
// queue infrastructure should be provisioned out-of-band so the worker
// doesn't try to declare exchanges / topics on every boot.
type MessengerAutoSetupCheck struct{}

func (MessengerAutoSetupCheck) ID() string { return "messenger-auto-setup" }

func (MessengerAutoSetupCheck) Run(cfg *symfonyconfig.Config) []Result {
	var out []Result
	for _, name := range []string{
		"MESSENGER_TRANSPORT_DSN",
		"MESSENGER_TRANSPORT_LOW_PRIORITY_DSN",
		"MESSENGER_TRANSPORT_FAILURE_DSN",
	} {
		dsn, ok := cfg.LookupEnv(name)
		if !ok || dsn == "" {
			continue
		}
		if !isAutoSetupEnabled(dsn) {
			continue
		}
		out = append(out, Result{
			ID:       "messenger-auto-setup-" + strings.ToLower(name),
			Title:    "Messenger transport runs auto_setup on every boot",
			Message:  fmt.Sprintf("%s has auto_setup enabled (defaults to true if unset). Append ?auto_setup=false in production to skip the per-boot topology check.", name),
			Severity: SeverityInfo,
			DocURL:   "https://developer.shopware.com/docs/guides/hosting/infrastructure/message-queue.html",
			Path:     name,
		})
	}
	return out
}

func isAutoSetupEnabled(dsn string) bool {
	// DSNs like "redis://..." parse cleanly; only the query string matters.
	q := ""
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		q = dsn[idx+1:]
	}
	if q == "" {
		// Symfony / Shopware default for transports is auto_setup=true.
		return true
	}
	values, err := url.ParseQuery(q)
	if err != nil {
		return true
	}
	v, ok := values["auto_setup"]
	if !ok || len(v) == 0 {
		return true
	}
	switch strings.ToLower(v[0]) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// MessengerTransportCheck flags Messenger transport DSNs that use
// non-production schemes (doctrine, sync). These don't scale to multiple
// workers and should be redis/rabbitmq in production.
type MessengerTransportCheck struct{}

func (MessengerTransportCheck) ID() string { return "messenger-transport" }

func (MessengerTransportCheck) Run(cfg *symfonyconfig.Config) []Result {
	var out []Result
	for _, name := range []string{
		"MESSENGER_TRANSPORT_DSN",
		"MESSENGER_TRANSPORT_LOW_PRIORITY_DSN",
	} {
		dsn, ok := cfg.LookupEnv(name)
		if !ok || dsn == "" {
			continue
		}
		scheme := dsnScheme(dsn)
		switch scheme {
		case "doctrine":
			out = append(out, Result{
				ID:       "messenger-transport-doctrine-" + strings.ToLower(name),
				Title:    "Messenger transport uses the database",
				Message:  fmt.Sprintf("%s uses the doctrine:// transport. The DB queue doesn't scale well across multiple workers; switch to redis:// or rabbitmq://.", name),
				Severity: SeverityWarning,
				DocURL:   "https://developer.shopware.com/docs/guides/hosting/infrastructure/message-queue.html",
				Path:     name,
			})
		case "sync":
			out = append(out, Result{
				ID:       "messenger-transport-sync-" + strings.ToLower(name),
				Title:    "Messenger transport is sync (no real queue)",
				Message:  fmt.Sprintf("%s uses the sync:// transport. Messages run inline and block the request — not suitable for production. Use redis:// or rabbitmq://.", name),
				Severity: SeverityError,
				DocURL:   "https://developer.shopware.com/docs/guides/hosting/infrastructure/message-queue.html",
				Path:     name,
			})
		}
	}
	return out
}

func dsnScheme(dsn string) string {
	idx := strings.Index(dsn, "://")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(dsn[:idx])
}
