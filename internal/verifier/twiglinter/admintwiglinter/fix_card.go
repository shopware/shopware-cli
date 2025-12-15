package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type CardFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(CardFixer{})
}

func (c CardFixer) Check(nodes []html.Node) []validation.CheckResult {
	// ...existing code...
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-card" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-card is removed, use mt-card instead. Review conversion for aiBadge and contentPadding.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-card",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (c CardFixer) Supports(v *version.Version) bool {
	// ...existing code...
	return twiglinter.Shopware67Constraint.Check(v)
}

func (c CardFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-card" {
			node.Tag = "mt-card"
			var newAttrs html.NodeList
			aiBadgeFound := false
			// Process attributes: remove aiBadge and contentPadding.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch attr.Key {
					case "aiBadge", "contentPadding":
						if attr.Key == "aiBadge" {
							aiBadgeFound = true
						}
						// remove attribute
					default:
						newAttrs = append(newAttrs, attr)
					}
				} else {
					// If it's not an html.Attribute (e.g., TwigIfNode), preserve it as is
					newAttrs = append(newAttrs, attrNode)
				}
			}
			node.Attributes = newAttrs

			// If aiBadge was present, add title slot with sw-ai-copilot-badge.
			if aiBadgeFound {
				aiBadgeSlot := &html.ElementNode{
					Tag: "slot",
					Attributes: html.NodeList{
						html.Attribute{Key: "name", Value: "title"},
					},
					Children: html.NodeList{
						&html.ElementNode{Tag: "sw-ai-copilot-badge"},
					},
				}
				// Prepend the title slot to existing children.
				node.Children = append(html.NodeList{aiBadgeSlot}, node.Children...)
			}
		}
	})
	return nil
}
