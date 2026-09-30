package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type TextareaFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(TextareaFieldFixer{})
}

func (t TextareaFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var checkErrors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-textarea-field" {
			checkErrors = append(checkErrors, validation.CheckResult{
				Message:    "sw-textarea-field is deprecated, use mt-textarea instead. Please manually review the new API differences.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-textarea-field",
				Line:       node.Line,
			})
		}
	})
	return checkErrors
}

func (t TextareaFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (t TextareaFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-textarea-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-textarea"
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
