package twiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
)

var Shopware67Constraint = version.MustConstraints(version.NewConstraint(">=6.7.0"))

const TwigExtension = ".twig"

var availableStorefrontFixers = []TwigFixer{}

var availableAdministrationFixers = []TwigFixer{}

func AddStorefrontFixer(fixer TwigFixer) {
	availableStorefrontFixers = append(availableStorefrontFixers, fixer)
}

func AddAdministrationFixer(fixer TwigFixer) {
	availableAdministrationFixers = append(availableAdministrationFixers, fixer)
}

func GetStorefrontFixers(version *version.Version) []TwigFixer {
	fixers := []TwigFixer{}
	for _, fixer := range availableStorefrontFixers {
		if fixer.Supports(version) {
			fixers = append(fixers, fixer)
		}
	}

	return fixers
}

func GetAdministrationFixers(version *version.Version) []TwigFixer {
	fixers := []TwigFixer{}
	for _, fixer := range availableAdministrationFixers {
		if fixer.Supports(version) {
			fixers = append(fixers, fixer)
		}
	}

	return fixers
}

type TwigFixer interface {
	Check(node []html.Node) []validation.CheckResult
	Supports(version *version.Version) bool
	Fix(node []html.Node) error
}
