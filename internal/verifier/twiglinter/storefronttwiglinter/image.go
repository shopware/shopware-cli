package storefronttwiglinter

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type ImageAltCheck struct{}

func (i ImageAltCheck) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag != "img" {
			return
		}

		var hasAlt bool
		var altValue string

		for _, attr := range node.Attributes {
			attrElement, ok := attr.(html.Attribute)
			if !ok {
				continue
			}

			if attrElement.Key == "alt" {
				hasAlt = true
				altValue = strings.TrimSpace(attrElement.Value)
				break
			}
		}

		if !hasAlt {
			errors = append(errors, validation.CheckResult{
				Message:    "Image tags must have an alt attribute for accessibility",
				Severity:   validation.SeverityWarning,
				Identifier: "twig-linter/image-missing-alt",
				Line:       node.Line,
			})
		} else if altValue == "" {
			errors = append(errors, validation.CheckResult{
				Message:    "Image alt attribute should not be empty - provide meaningful description or use empty alt=\"\" for decorative images",
				Severity:   validation.SeverityWarning,
				Identifier: "twig-linter/image-empty-alt",
				Line:       node.Line,
			})
		}
	})

	return errors
}

func (i ImageAltCheck) Supports(v *version.Version) bool {
	return true
}

func (i ImageAltCheck) Fix(nodes []html.Node) error {
	return nil // No automatic fix for alt attributes, requires manual intervention
}

func init() {
	twiglinter.AddStorefrontFixer(ImageAltCheck{})
}
