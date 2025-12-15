package admintwiglinter

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type IconFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(IconFixer{})
}

func (i IconFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-icon" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-icon is removed, use mt-icon instead with proper size prop.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-icon",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (i IconFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (i IconFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-icon" {
			node.Tag = "mt-icon"
			hasSize := false
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch strings.ToLower(attr.Key) {
					case "small":
						// Replace "small" with size="16px"
						newAttrs = append(newAttrs, html.Attribute{
							Key:   "size",
							Value: "16px",
						})
						hasSize = true
					case "large":
						// Replace "large" with size="32px"
						newAttrs = append(newAttrs, html.Attribute{
							Key:   "size",
							Value: "32px",
						})
						hasSize = true
					case "size":
						// keep existing size prop
						newAttrs = append(newAttrs, attr)
						hasSize = true
					default:
						newAttrs = append(newAttrs, attr)
					}
				} else {
					// If it's not an html.Attribute (e.g., TwigIfNode), preserve it as is
					newAttrs = append(newAttrs, attrNode)
				}
			}

			// If no size related prop is set, add default size="24px"
			if !hasSize {
				newAttrs = append(newAttrs, html.Attribute{
					Key:   "size",
					Value: "24px",
				})
			}
			node.Attributes = newAttrs
		}
	})
	return nil
}
