package configcheck

import (
	"fmt"
	"strings"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// MonologHandlerLevelCheck inspects monolog.handlers.* and flags handlers
// configured below WARNING. Verbose logging on the hot path is one of the
// most common performance regressions in Shopware production deployments.
type MonologHandlerLevelCheck struct{}

func (MonologHandlerLevelCheck) ID() string { return "monolog-handler-level" }

func (MonologHandlerLevelCheck) Run(cfg *symfonyconfig.Config) []Result {
	raw, ok := cfg.Get("monolog.handlers")
	if !ok {
		return nil
	}
	handlers, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	var out []Result
	for name, h := range handlers {
		handler, ok := h.(map[string]any)
		if !ok {
			continue
		}
		levelRaw, ok := handler["level"]
		if !ok {
			continue
		}
		level, ok := levelRaw.(string)
		if !ok {
			continue
		}
		level = cfg.Resolve(level)
		if !isBelowWarning(level) {
			continue
		}
		out = append(out, Result{
			ID:       "monolog-handler-level-" + name,
			Title:    "Monolog handler logs below WARNING",
			Message:  fmt.Sprintf("monolog.handlers.%s.level is %q. Raise it to WARNING or higher in production to avoid logging every notice on the hot path.", name, level),
			Severity: SeverityInfo,
			DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
			Path:     "monolog.handlers." + name + ".level",
		})
	}
	return out
}

// isBelowWarning reports whether the given monolog level name sits below
// WARNING. We accept either the symbolic name (case-insensitive) or the
// numeric value the Symfony env(int:...) processor would emit.
func isBelowWarning(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "notice", "100", "200", "250":
		return true
	}
	return false
}
