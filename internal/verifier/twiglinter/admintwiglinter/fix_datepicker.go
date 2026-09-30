package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type DatepickerFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(DatepickerFixer{})
}

func (d DatepickerFixer) Check(nodes []html.Node) []validation.CheckResult {
	var checkErrors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-datepicker" {
			checkErrors = append(checkErrors, validation.CheckResult{
				Message:    "sw-datepicker is deprecated, use mt-datepicker instead. Please review the conversion for the label property.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-datepicker",
				Line:       node.Line,
			})
		}
	})
	return checkErrors
}

func (d DatepickerFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (d DatepickerFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-datepicker" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-datepicker"

			var newAttrs html.NodeList
			// Update attribute names.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ColonValueAttr:
						attr.Key = ":model-value"
						newAttrs = append(newAttrs, attr)
					case VModelValueAttr:
						attr.Key = VModelAttr
						newAttrs = append(newAttrs, attr)
					case UpdateValueAttr:
						attr.Key = "@update:model-value"
						newAttrs = append(newAttrs, attr)
					default:
						newAttrs = append(newAttrs, attr)
					}
				} else {
					// If it's not an html.Attribute (e.g., TwigIfNode), preserve it as is
					newAttrs = append(newAttrs, attrNode)
				}
			}
			node.Attributes = newAttrs

			convertFieldSlots(node, "label")
		}
	})

	return nil
}
