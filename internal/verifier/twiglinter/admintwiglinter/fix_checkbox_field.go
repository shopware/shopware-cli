package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type CheckboxFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(CheckboxFieldFixer{})
}

func (c CheckboxFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	// ...existing code...
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-checkbox-field" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-checkbox-field is deprecated, use mt-checkbox instead. Review conversion for props, events and slots.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-checkbox-field",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (c CheckboxFieldFixer) Supports(v *version.Version) bool {
	// ...existing code...
	return twiglinter.Shopware67Constraint.Check(v)
}

func (c CheckboxFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-checkbox-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-checkbox"
			var newAttrs html.NodeList
			// Process attribute conversions.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ColonValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: ":checked", Value: attr.Value})
					case VModelAttr, VModelValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: "v-model:checked", Value: attr.Value})
					case "id", "ghostValue", "padded":
						// remove these attributes without replacement
					case "partlyChecked":
						newAttrs = append(newAttrs, &html.Attribute{Key: "partial"})
					case UpdateValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: "@update:checked", Value: attr.Value})
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
