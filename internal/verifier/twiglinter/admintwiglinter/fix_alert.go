package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type AlertFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(AlertFixer{})
}

func (a AlertFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-alert" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-alert is removed, use mt-banner instead. Please review conversion for variant changes.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-alert",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (a AlertFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (a AlertFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-alert" {
			node.Tag = "mt-banner"
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					if attr.Key == "variant" {
						switch attr.Value {
						case "success":
							attr.Value = "positive"
							newAttrs = append(newAttrs, attr)
						case "error":
							attr.Value = "critical"
							newAttrs = append(newAttrs, attr)
						case "warning":
							attr.Value = "attention"
							newAttrs = append(newAttrs, attr)
						case "info":
							// Keep info as is
							newAttrs = append(newAttrs, attr)
						default:
							// Keep any other variants unchanged
							newAttrs = append(newAttrs, attr)
						}
					} else {
						// Preserve all other attributes
						newAttrs = append(newAttrs, attr)
					}
				} else {
					// If it's not an html.Attribute (e.g., TwigIfNode), preserve it as is
					newAttrs = append(newAttrs, attrNode)
				}
			}

			node.Attributes = newAttrs
		}
	})
	return nil
}
