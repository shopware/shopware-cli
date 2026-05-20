package configcheck

import (
	"fmt"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// FineGrainedCachingCheck flags shopware.cache.tagging.each_* options. They
// generate large numbers of small cache tags which is costly on Redis or
// Varnish. The feature was removed in Shopware 6.7.0 so this check is
// primarily relevant on 6.5.4 - 6.6.x.
type FineGrainedCachingCheck struct{}

func (FineGrainedCachingCheck) ID() string { return "fine-grained-caching" }

func (FineGrainedCachingCheck) Run(cfg *symfonyconfig.Config) []Result {
	var out []Result
	for _, key := range []string{"each_config", "each_snippet", "each_theme_config"} {
		path := fmt.Sprintf("shopware.cache.tagging.%s", key)
		v, ok := cfg.GetBool(path)
		if !ok || !v {
			continue
		}
		out = append(out, Result{
			ID:       "fine-grained-caching-" + key,
			Title:    "Fine-grained cache tagging hurts cache backends",
			Message:  fmt.Sprintf("%s is true. Set it to false to reduce per-request cache tag traffic to Redis/Varnish.", path),
			Severity: SeverityInfo,
			DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
			Path:     path,
		})
	}
	return out
}
