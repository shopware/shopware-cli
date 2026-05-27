package packagist

import (
	"strings"

	"github.com/shyim/go-version"
)

// ConstraintsSatisfiedBy reports whether every constraint that requires
// declares for a package named in packages is satisfied by target.
// Constraints for packages not listed in packages are ignored, and packages
// that declare no constraint are treated as satisfied. An unparseable
// constraint is treated as not satisfied.
func ConstraintsSatisfiedBy(requires map[string]string, packages []string, target *version.Version) bool {
	for _, name := range packages {
		constraint, ok := requires[name]
		if !ok {
			continue
		}

		c, err := version.NewConstraint(constraint)
		if err != nil {
			return false
		}

		if !c.Check(target) {
			return false
		}
	}

	return true
}

// BumpConstraint turns a concrete version (e.g. "2.3.4") into a caret
// constraint ("^2.3.4") suitable for a composer.json require entry. Values
// that already look like a constraint (containing range/wildcard operators)
// are returned unchanged.
func BumpConstraint(version string) string {
	if version == "" {
		return version
	}

	if strings.ContainsAny(version, "^~><*|, ") {
		return version
	}

	return "^" + version
}
