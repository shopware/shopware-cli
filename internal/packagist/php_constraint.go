package packagist

import (
	"strings"

	"github.com/shyim/go-version"
)

// SupportedPHPVersions lists the PHP versions selectable for the docker dev image,
// ordered from lowest to highest.
var SupportedPHPVersions = []string{"8.2", "8.3", "8.4", "8.5"}

// PHPConstraint represents one or more composer-style `php` constraints (e.g. "^8.2"
// or "~8.2.0 || ~8.3.0"). A nil receiver is treated as "no constraint" and matches
// any supported PHP version.
type PHPConstraint struct {
	raw []string
}

// NewPHPConstraint constructs a PHPConstraint from one or more composer-style
// constraint strings. Empty strings are accepted and treated as "no constraint".
func NewPHPConstraint(constraints ...string) *PHPConstraint {
	return &PHPConstraint{raw: constraints}
}

// HighestSupported returns the highest entry from SupportedPHPVersions that satisfies
// every constraint. Invalid or empty constraints are ignored. A nil receiver returns
// the highest supported version. If no version satisfies all constraints, the highest
// supported version is returned as a best-effort default.
func (c *PHPConstraint) HighestSupported() string {
	parsed := c.parse()

	for i := len(SupportedPHPVersions) - 1; i >= 0; i-- {
		candidate := SupportedPHPVersions[i]
		v, err := version.NewVersion(candidate + ".0")
		if err != nil {
			continue
		}
		if satisfiesAll(v, parsed) {
			return candidate
		}
	}

	return SupportedPHPVersions[len(SupportedPHPVersions)-1]
}

// Check reports whether the given PHP version (e.g. "8.3.7") satisfies every
// constraint. A nil receiver always returns true. An unparseable version returns
// false.
func (c *PHPConstraint) Check(phpVersion string) bool {
	v, err := version.NewVersion(phpVersion)
	if err != nil {
		return false
	}
	return satisfiesAll(v, c.parse())
}

// String returns the raw composer-style constraint strings joined by ", " for display
// purposes. Empty constraints are omitted. A nil receiver returns an empty string.
func (c *PHPConstraint) String() string {
	if c == nil {
		return ""
	}
	parts := make([]string, 0, len(c.raw))
	for _, raw := range c.raw {
		if raw != "" {
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, ", ")
}

func (c *PHPConstraint) parse() []version.Constraints {
	if c == nil {
		return nil
	}
	parsed := make([]version.Constraints, 0, len(c.raw))
	for _, raw := range c.raw {
		if raw == "" {
			continue
		}
		cs, err := version.NewConstraint(raw)
		if err != nil {
			continue
		}
		parsed = append(parsed, cs)
	}
	return parsed
}

func satisfiesAll(v *version.Version, constraints []version.Constraints) bool {
	for _, c := range constraints {
		if !c.Check(v) {
			return false
		}
	}
	return true
}
