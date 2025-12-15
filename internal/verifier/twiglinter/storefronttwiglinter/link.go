package storefronttwiglinter

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type LinkCheck struct{}

func (l LinkCheck) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag != "a" {
			return
		}

		var href, target, rel string

		for _, attr := range node.Attributes {
			attrElement, ok := attr.(html.Attribute)
			if !ok {
				continue
			}

			switch attrElement.Key {
			case "href":
				href = attrElement.Value
			case "target":
				target = attrElement.Value
			case "rel":
				rel = attrElement.Value
			}
		}

		isExternal := strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")

		if !isExternal {
			return
		}

		hasBlank := target == "_blank"
		if !hasBlank {
			errors = append(errors, validation.CheckResult{
				Message:    "External links must have target=\"_blank\"",
				Severity:   validation.SeverityWarning,
				Identifier: "twig-linter/external-link-missing-target-blank",
				Line:       node.Line,
			})
		}

		hasNoOpener := false
		if rel != "" {
			rels := strings.Fields(rel)
			for _, r := range rels {
				if r == "noopener" {
					hasNoOpener = true
					break
				}
			}
		}

		if !hasNoOpener {
			errors = append(errors, validation.CheckResult{
				Message:    "External links must have rel=\"noopener\" for security reasons",
				Severity:   validation.SeverityWarning,
				Identifier: "twig-linter/external-link-missing-noopener",
				Line:       node.Line,
			})
		}
	})

	return errors
}

func (l LinkCheck) Supports(v *version.Version) bool {
	return true
}

func (l LinkCheck) Fix(nodes []html.Node) error {
	return nil
}

func init() {
	twiglinter.AddStorefrontFixer(LinkCheck{})
}
