package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type PasswordFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(PasswordFieldFixer{})
}

func (p PasswordFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var checkErrors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-password-field" {
			checkErrors = append(checkErrors, validation.CheckResult{
				Message:    "sw-password-field is deprecated, use mt-password-field instead. Review conversion for label/hint properties.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-password-field",
				Line:       node.Line,
			})
		}
	})
	return checkErrors
}

func (p PasswordFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (p PasswordFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-password-field" {
			if !canConvertFieldSlots(node, "label", "hint") {
				return
			}
			node.Tag = "mt-password-field"

			// Update or remove attributes
			var newAttrs html.NodeList
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ValueAttr, ColonValueAttr:
						attr.Key = migratedValueKey(attr.Key)
						newAttrs = append(newAttrs, attr)
					case VModelValueAttr:
						attr.Key = "v-model"
						newAttrs = append(newAttrs, attr)
					case "size":
						if attr.Value == "medium" {
							attr.Value = "default"
						}
						newAttrs = append(newAttrs, attr)
					case "isInvalid":
						// remove attribute
					case "@update:value":
						attr.Key = "@update:model-value"
						newAttrs = append(newAttrs, attr)
					case "@base-field-mounted":
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

			convertFieldSlots(node, "label", "hint")
		}
	})
	return nil
}
