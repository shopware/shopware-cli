package admintwiglinter

import (
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type NumberFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(NumberFieldFixer{})
}

func (n NumberFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-number-field" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-number-field is deprecated, use mt-number-field instead. Review conversion for props, events and slots; complex slots require manual migration.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-number-field",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (n NumberFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (n NumberFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-number-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-number-field"
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ColonValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{
							Key:   ":model-value",
							Value: attr.Value,
						})
					case VModelValueAttr:
						attr.Key = VModelAttr
						newAttrs = append(newAttrs, attr)
					case "@update:value":
						newAttrs = append(newAttrs, &html.Attribute{
							Key:   UpdateModelValueAttr,
							Value: attr.Value,
						})
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
