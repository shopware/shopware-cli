package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type UrlFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(UrlFieldFixer{})
}

func (u UrlFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-url-field" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-url-field is deprecated, use mt-url-field instead. Review conversion for props, events, label and hint slot.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-url-field",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (u UrlFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (u UrlFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-url-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-url-field"
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ValueAttr, ColonValueAttr:
						attr.Key = migratedValueKey(attr.Key)
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
