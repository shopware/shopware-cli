package admintwiglinter

import (
	"strings"

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
				Message:    "sw-switch-field is removed, use mt-switch instead. Review conversion for props, events and slots.",
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
			node.Tag = "mt-switch"
			var newAttrs html.NodeList
			// Process attribute conversions.
			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch attr.Key {
					case "noMarginTop":
						newAttrs = append(newAttrs, html.Attribute{Key: "removeTopMargin"})
					case SizeAttr, "id", "ghostValue", "padded", "partlyChecked":
						// remove these attributes
					case ValueAttr:
						newAttrs = append(newAttrs, html.Attribute{Key: "checked", Value: attr.Value})
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

			// Process children for slot conversion.
			var labelText string
			var remainingChildren html.NodeList
			for _, child := range node.Children {
				// Check if child is a slot element.
				if elem, ok := child.(*html.ElementNode); ok && elem.Tag == TemplateTag {
					for _, a := range elem.Attributes {
						if attr, ok := a.(html.Attribute); ok {
							if attr.Key == LabelSlotAttr {
								var sb strings.Builder
								for _, inner := range elem.Children {
									sb.WriteString(strings.TrimSpace(inner.Dump(0)))
								}
								labelText = sb.String()
								goto SkipChild
							}
							if attr.Key == HintSlotAttr {
								goto SkipChild
							}
						}
					}
				}
				remainingChildren = append(remainingChildren, child)
			SkipChild:
			}
			// Remove all slot children.
			node.Children = remainingChildren
			// If label slot found, add label attribute.
			if labelText != "" {
				node.Attributes = append(node.Attributes, html.Attribute{
					Key:   "label",
					Value: labelText,
				})
			}
		}
	})
	return nil
}
