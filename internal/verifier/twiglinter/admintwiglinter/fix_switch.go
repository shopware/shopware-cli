package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type SwitchFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(SwitchFixer{})
}

func (s SwitchFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-switch-field" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-switch-field is deprecated, use mt-switch instead. Review conversion for props, events and slots.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-switch-field",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (s SwitchFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (s SwitchFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-switch-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-switch"
			var newAttrs html.NodeList
			// Process attribute conversions.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case "noMarginTop":
						newAttrs = append(newAttrs, &html.Attribute{Key: "removeTopMargin"})
					case SizeAttr, "id", "ghostValue", "padded", "partlyChecked":
						// remove these attributes
					case ValueAttr, ColonValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: migratedValueKey(attr.Key), Value: attr.Value})
					case UpdateValueAttr:
						attr.Key = UpdateModelValueAttr
						newAttrs = append(newAttrs, attr)
					case VModelValueAttr:
						attr.Key = VModelAttr
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
