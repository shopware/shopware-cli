package admintwiglinter

import (
	"strings"

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
				Message:    "sw-url-field is removed, use mt-url-field instead. Review conversion for props, events, label and hint slot.",
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
			node.Tag = "mt-url-field"
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch attr.Key {
					case ValueAttr:
						attr.Key = ModelValueAttr
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

			// Process slot conversion.
			label := ""
			var remainingChildren html.NodeList
			for _, child := range node.Children {
				if elem, ok := child.(*html.ElementNode); ok && elem.Tag == TemplateTag {
					for _, a := range elem.Attributes {
						if attr, ok := a.(html.Attribute); ok {
							if attr.Key == LabelSlotAttr {
								var sb strings.Builder
								for _, inner := range elem.Children {
									sb.WriteString(strings.TrimSpace(inner.Dump(0)))
								}
								label = sb.String()
								goto SkipChild
							}
							if attr.Key == HintSlotAttr {
								// Skip hint slot.
								goto SkipChild
							}
						}
					}
				}
				remainingChildren = append(remainingChildren, child)
			SkipChild:
			}
			// Remove all children; label was processed, and hint slot is dropped.
			node.Children = remainingChildren
			if label != "" {
				node.Attributes = append(node.Attributes, html.Attribute{
					Key:   "label",
					Value: label,
				})
			}
		}
	})
	return nil
}
