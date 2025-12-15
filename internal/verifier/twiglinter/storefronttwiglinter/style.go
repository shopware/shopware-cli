package storefronttwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type StyleFixer struct{}

func (s StyleFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "style" {
			errors = append(errors, validation.CheckResult{
				Message:    "Prefer to use dedicated CSS files instead of inline style tag",
				Severity:   validation.SeverityWarning,
				Identifier: "twig-linter/inline-style-tag",
				Line:       node.Line,
			})
		}

		for _, attr := range node.Attributes {
			attrElement, ok := attr.(html.Attribute)

			if ok && attrElement.Key == "style" {
				errors = append(errors, validation.CheckResult{
					Message:    "Prefer to use CSS classes instead of inline styling",
					Severity:   validation.SeverityWarning,
					Identifier: "twig-linter/inline-style-attribute",
					Line:       node.Line,
				})
			}
		}
	})

	return errors
}

func (s StyleFixer) Supports(v *version.Version) bool {
	return true
}

func (s StyleFixer) Fix(nodes []html.Node) error {
	return nil // No automatic fix for inline styles, requires manual intervention
}

func init() {
	twiglinter.AddStorefrontFixer(StyleFixer{})
}
