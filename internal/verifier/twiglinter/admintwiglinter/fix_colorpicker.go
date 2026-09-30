package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type ColorpickerFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(ColorpickerFixer{})
}

func (c ColorpickerFixer) Check(nodes []html.Node) []validation.CheckResult {
	var checkErrors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-colorpicker" {
			checkErrors = append(checkErrors, validation.CheckResult{
				Message:    "sw-colorpicker is deprecated, use mt-colorpicker instead. Review conversion for label property.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-colorpicker",
				Line:       node.Line,
			})
		}
	})
	return checkErrors
}

func (c ColorpickerFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (c ColorpickerFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-colorpicker" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-colorpicker"

			var newAttrs html.NodeList
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
						attr.Key = UpdateModelValueAttr
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
