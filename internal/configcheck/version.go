package configcheck

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// shopwareVersionMatches reports whether cfg.ShopwareVersion satisfies the
// given composer-style constraint (e.g. ">=6.6.4.0 <6.7.1.0").
//
// When the version is unknown (no composer.lock, no override), the answer is
// false - version-gated checks should not fire on projects where we can't
// confirm they apply.
//
// Constraint strings must use space-separated AND form (the comma form
// fails to parse in shyim/go-version). An invalid constraint panics rather
// than silently disabling the check, so a malformed rule definition surfaces
// during tests instead of in production.
func shopwareVersionMatches(cfg *symfonyconfig.Config, constraint string) bool {
	if cfg.ShopwareVersion == nil {
		return false
	}
	c, err := version.NewConstraint(constraint)
	if err != nil {
		panic("configcheck: invalid version constraint " + constraint + ": " + err.Error())
	}
	return c.Check(cfg.ShopwareVersion)
}
