package admintwiglinter

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type EmailFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(EmailFieldFixer{})
}

func (e EmailFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-email-field" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-email-field is removed, use mt-email-field instead. Review conversion for props, events and label slot.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-email-field",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (e EmailFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (e EmailFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-email-field" {
			node.Tag = "mt-email-field"
			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch attr.Key {
					case ValueAttr:
						attr.Key = "model-value"
						newAttrs = append(newAttrs, attr)
					case VModelValueAttr:
						attr.Key = "v-model"
						newAttrs = append(newAttrs, attr)
					case SizeAttr:
						if attr.Value == MediumValue {
							attr.Value = DefaultValue
						}
						newAttrs = append(newAttrs, attr)
					case IsInvalidAttr, AiBadgeAttr, BaseFieldMountedAttr:
						// remove attribute
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

			// Process label slot.
			label := ""
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
						}
					}
				}
			SkipChild:
			}
			node.Children = nil
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
