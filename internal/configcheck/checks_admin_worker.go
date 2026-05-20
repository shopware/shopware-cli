package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// AdminWorkerCheck flags shopware.admin_worker.enable_admin_worker = true.
// The admin worker polls the message queue from the admin UI, which doesn't
// scale: production deployments should run dedicated CLI workers instead.
type AdminWorkerCheck struct{}

func (AdminWorkerCheck) ID() string { return "admin-worker-enabled" }

func (AdminWorkerCheck) Run(cfg *symfonyconfig.Config) []Result {
	const path = "shopware.admin_worker.enable_admin_worker"
	v, ok := cfg.GetBool(path)
	if !ok || !v {
		return nil
	}
	return []Result{{
		ID:       "admin-worker-enabled",
		Title:    "Admin worker is enabled",
		Message:  "shopware.admin_worker.enable_admin_worker is true. Disable it and run a dedicated CLI worker (`bin/console messenger:consume`) for production-grade message queue throughput.",
		Severity: SeverityWarning,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/infrastructure/message-queue.html",
		Path:     path,
	}}
}
