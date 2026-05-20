// Package configcheck runs recommendation rules against a merged Symfony
// configuration. Rules are ported from FriendsOfShopware/FroshTools'
// PerformanceChecker classes and read the same configuration values, but
// from the static config files in config/packages and .env files rather
// than from a running Shopware container.
package configcheck

import (
	"sort"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// Severity classifies how strongly we recommend acting on a finding.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	return "unknown"
}

// Result describes a single recommendation produced by a Check.
type Result struct {
	// ID is a stable, machine-readable identifier (e.g. "admin-worker-enabled").
	ID string
	// Title is a short human-readable summary.
	Title string
	// Message explains the problem and the recommended fix.
	Message string
	// Severity ranks the finding.
	Severity Severity
	// DocURL is an optional pointer at upstream documentation.
	DocURL string
	// Path is the dotted config path or env-var name the finding refers to.
	// Empty when the finding is not tied to a specific value.
	Path string
}

// Check inspects a merged Symfony configuration and returns any matching
// recommendations. Returning a nil slice means "no recommendation"; returning
// multiple results is allowed when a single rule covers several values
// (e.g. fine-grained caching keys).
type Check interface {
	ID() string
	Run(cfg *symfonyconfig.Config) []Result
}

// Run executes every check against cfg and returns all findings, sorted by
// (severity descending, ID ascending) so callers get a stable order.
func Run(cfg *symfonyconfig.Config, checks []Check) []Result {
	var out []Result
	for _, c := range checks {
		out = append(out, c.Run(cfg)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Default returns every built-in check. Callers are free to compose their
// own slice instead.
func Default() []Check {
	return []Check{
		AdminWorkerCheck{},
		IncrementStorageCheck{},
		FineGrainedCachingCheck{},
		DisabledMailUpdatesCheck{},
		AppUrlCheckDisabledCheck{},
		MessengerAutoSetupCheck{},
		MessengerTransportCheck{},
		CacheCompressionCheck{},
		CartCompressionCheck{},
		ProductStreamIndexingCheck{},
		ElasticsearchCheck{},
		MonologHandlerLevelCheck{},
	}
}
