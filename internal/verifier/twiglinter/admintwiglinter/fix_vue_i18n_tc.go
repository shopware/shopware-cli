package admintwiglinter

import (
	"regexp"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

// tcCallRegexp matches the Vue i18n `$tc` translation function so it can be
// rewritten to `$t`. The trailing word boundary keeps identifiers such as
// `$tcFoo` untouched while still matching `$tc(`, `$tc (` and `this.$tc(`.
var tcCallRegexp = regexp.MustCompile(`\$tc\b`)

// VueI18nTcFixer rewrites the deprecated Vue i18n `$tc` translation function to
// `$t`. `$tc` is deprecated and will be removed in Shopware 6.8.0; `$t` handles
// all translations including pluralization.
type VueI18nTcFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(VueI18nTcFixer{})
}

func (f VueI18nTcFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	walkVueExpressions(nodes, func(expr string, line int) string {
		if tcCallRegexp.MatchString(expr) {
			errs = append(errs, validation.CheckResult{
				Message:    "The Vue i18n $tc function is deprecated and will be removed in Shopware 6.8.0, use $t instead.",
				Severity:   validation.SeverityWarning,
				Identifier: "vue-i18n-tc",
				Line:       line,
			})
		}
		return expr
	})
	return errs
}

func (f VueI18nTcFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (f VueI18nTcFixer) Fix(nodes []html.Node) error {
	walkVueExpressions(nodes, func(expr string, _ int) string {
		return tcCallRegexp.ReplaceAllLiteralString(expr, "$t")
	})
	return nil
}

// walkVueExpressions recursively walks the AST and invokes fn on every string
// that may hold a Vue expression: `{{ ... }}` interpolations and the values of
// bound attributes (`:foo`, `@foo`, `v-foo`). The string returned by fn
// replaces the original, so the same walk powers both Check (returns the value
// unchanged) and Fix (returns the rewritten value).
func walkVueExpressions(nodes []html.Node, fn func(expr string, line int) string) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *html.TemplateExpressionNode:
			n.Expression = fn(n.Expression, n.Line)
		case *html.ElementNode:
			for i, attrNode := range n.Attributes {
				attr, ok := attrNode.(html.Attribute)
				if !ok || !isBoundAttribute(attr.Key) {
					continue
				}
				attr.Value = fn(attr.Value, n.Line)
				n.Attributes[i] = attr
			}
			// Attributes can embed structural Twig nodes (e.g. {% if %}), so
			// recurse into them as well as the element's children.
			walkVueExpressions(n.Attributes, fn)
			walkVueExpressions(n.Children, fn)
		case *html.TwigBlockNode:
			walkVueExpressions(n.Children, fn)
		case *html.TwigIfNode:
			for bi := range n.Branches {
				walkVueExpressions(n.Branches[bi].Body, fn)
			}
			walkVueExpressions(n.ElseChildren, fn)
		case *html.TwigGenericBlockNode:
			walkVueExpressions(n.Body, fn)
			walkVueExpressions(n.Else, fn)
		}
	}
}

// isBoundAttribute reports whether an attribute key holds a Vue expression
// rather than a static string value: bindings (`:foo`), event handlers
// (`@foo`) and directives (`v-foo`).
func isBoundAttribute(key string) bool {
	if key == "" {
		return false
	}
	switch key[0] {
	case ':', '@':
		return true
	}
	return len(key) > 2 && key[0] == 'v' && key[1] == '-'
}
