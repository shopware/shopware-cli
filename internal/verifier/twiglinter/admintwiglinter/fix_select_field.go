package admintwiglinter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type SelectFieldFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(SelectFieldFixer{})
}

func (s SelectFieldFixer) Check(nodes []html.Node) []validation.CheckResult {
	var errs []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-select-field" {
			errs = append(errs, validation.CheckResult{
				Message:    "sw-select-field is deprecated, use mt-select instead. Complex slots and options expressions require manual migration; review props and events.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-select-field",
				Line:       node.Line,
			})
		}
	})
	return errs
}

func (s SelectFieldFixer) Supports(v *version.Version) bool {
	return twiglinter.Shopware67Constraint.Check(v)
}

func (s SelectFieldFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-select-field" {
			// Dynamic options and arbitrary JavaScript option objects require manual migration.
			if !canConvertSelect(node) {
				return
			}
			node.Tag = "mt-select"

			var newAttrs html.NodeList

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(*html.Attribute); ok {
					switch attr.Key {
					case ColonValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: ":model-value", Value: attr.Value})
					case VModelValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: "v-model", Value: attr.Value})
					case ":aside":
						// Remove aside prop.
					case UpdateValueAttr:
						newAttrs = append(newAttrs, &html.Attribute{Key: UpdateModelValueAttr, Value: attr.Value})
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

			// Process children for slot conversion.
			var optionObjects []map[string]interface{}
			var expressionOptions = make(map[string]string)
			var expressionObjectPrefix = "abc541d6050-b044-4de0-9edd-cad83c4f3365-"
			var expressionObjectKey = 0

			for _, child := range node.Children {
				if elem, ok := child.(*html.ElementNode); ok {
					// Collect <option> children from default slot.
					if elem.Tag == "option" {
						opt := make(map[string]interface{})
						// Get option value from attributes.
						for _, a := range elem.Attributes {
							if attr, ok := a.(*html.Attribute); ok {
								switch attr.Key {
								case ColonValueAttr, VModelValueAttr:
									expressionKey := fmt.Sprintf("%s:%d", expressionObjectPrefix, expressionObjectKey)
									expressionOptions[expressionKey] = attr.Value
									opt["value"] = expressionKey
									expressionObjectKey++
								case "value":
									opt["value"] = attr.Value
								}
							}
						}
						// Get option label from inner text.
						var sb strings.Builder
						for _, inner := range elem.Children {
							sb.WriteString(strings.TrimSpace(inner.Dump(0)))
						}

						label := sb.String()

						if strings.HasPrefix(label, "{{") && strings.HasSuffix(label, "}}") {
							expressionKey := fmt.Sprintf("%s:%d", expressionObjectPrefix, expressionObjectKey)
							expressionOptions[expressionKey] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(label, "}}"), "{{"))
							label = expressionKey
							expressionObjectKey++
						}

						opt["label"] = label
						optionObjects = append(optionObjects, opt)
						goto SkipChild
					}
				}
			SkipChild:
			}
			// Remove all children slots.
			node.Children = nil

			// If default <option> elements were found, build options prop.
			if len(optionObjects) > 0 {
				// Serialize optionObjects slice to JSON-like string.
				bytes, err := json.Marshal(optionObjects)
				if err == nil {
					json := string(bytes)

					for replacementKey, expression := range expressionOptions {
						json = strings.ReplaceAll(json, "\""+replacementKey+"\"", fmt.Sprintf("(%s)", expression))
					}

					node.Attributes = append(node.Attributes, &html.Attribute{
						Key:   ":options",
						Value: json,
					})
				}
			}
		}
	})
	return nil
}

// Leave unsupported templates intact so the checker continues to report them.
func canConvertSelect(node *html.ElementNode) bool {
	if !canConvertFieldSlots(node, "label") {
		return false
	}
	for _, a := range node.Attributes {
		attr, ok := a.(*html.Attribute)
		if !ok {
			return false
		}
		if attr.Key == ":options" || attr.Key == "options" {
			return false
		}
	}
	for _, child := range node.Children {
		switch n := child.(type) {
		case *html.RawNode:
			if strings.TrimSpace(n.Text) != "" {
				return false
			}
		case *html.ElementNode:
			if slotName(n) == "label" {
				continue
			}
			if n.Tag != "option" {
				return false
			}
			for _, a := range n.Attributes {
				attr, ok := a.(*html.Attribute)
				if !ok {
					return false
				}
				switch attr.Key {
				case ValueAttr, ColonValueAttr, VModelValueAttr:
				default:
					return false
				}
			}
			if _, _, safe := slotValue(n.Children); !safe {
				return false
			}
		default:
			return false
		}
	}
	return true
}
