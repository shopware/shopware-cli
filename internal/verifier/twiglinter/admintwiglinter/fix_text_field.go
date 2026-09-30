package admintwiglinter

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type TextFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(TextFieldFixer{})
}

func (t TextFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-text-field" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-text-field is deprecated, use mt-text-field instead. Review conversion for props, events and slots; complex slots require manual migration.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-text-field",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (t TextFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (t TextFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-text-field" {
			if !canConvertFieldSlots(node, "label") {
				return
			}
			node.Tag = "mt-text-field"
			var newAttrs html.NodeList
			// Process attributes conversion.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ValueAttr:
						attr.Key = ModelValueAttr
						newAttrs = append(newAttrs, attr)
					case ColonValueAttr:
						attr.Key = ":model-value"
						newAttrs = append(newAttrs, attr)
					case VModelValueAttr:
						attr.Key = VModelAttr
						newAttrs = append(newAttrs, attr)
					case SizeAttr:
						if attr.Value == MediumValue {
							attr.Value = DefaultValue
						}
						newAttrs = append(newAttrs, attr)
					case IsInvalidAttr, AiBadgeAttr, BaseFieldMountedAttr:
						// remove these attributes
					case UpdateValueAttr:
						attr.Key = UpdateModelValueAttr
						newAttrs = append(newAttrs, attr)
					// shorten false-default attributes
					case ":copyable", ":copyableTooltip", ":copyable-tooltip", ":disabled", ":required", ":isInherited", ":is-inherited", ":disableInheritanceToggle", ":disable-inheritance-toggle":
						if strings.ToLower(attr.Value) == "true" {
							attr.Key = strings.TrimPrefix(attr.Key, ":")
							attr.Value = ""
						}
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
