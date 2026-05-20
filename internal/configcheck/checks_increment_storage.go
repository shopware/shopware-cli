package configcheck

import (
	"fmt"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// IncrementStorageCheck flags shopware.increment.*.type set to "mysql".
// MySQL-backed increment storage is slow under load and the recommended
// alternatives are "array" or "redis".
type IncrementStorageCheck struct{}

func (IncrementStorageCheck) ID() string { return "increment-storage-mysql" }

func (IncrementStorageCheck) Run(cfg *symfonyconfig.Config) []Result {
	var out []Result
	for _, kind := range []string{"user_activity", "message_queue"} {
		path := fmt.Sprintf("shopware.increment.%s.type", kind)
		v, ok := cfg.GetString(path)
		if !ok || v != "mysql" {
			continue
		}
		out = append(out, Result{
			ID:       "increment-storage-mysql-" + kind,
			Title:    "MySQL increment storage is slow",
			Message:  fmt.Sprintf("%s is set to \"mysql\". Switch to \"array\" or \"redis\" for better performance.", path),
			Severity: SeverityWarning,
			DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
			Path:     path,
		})
	}
	return out
}
