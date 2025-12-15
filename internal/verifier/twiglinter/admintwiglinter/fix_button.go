package admintwiglinter

import (
	"fmt"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

type ButtonFixer struct{}

func init() {
	twiglinter.AddAdministrationFixer(ButtonFixer{})
}

func (b ButtonFixer) Check(nodes []html.Node) []validation.CheckResult {
	// ...existing code...
	var errors []validation.CheckResult
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-button" {
			errors = append(errors, validation.CheckResult{
				Message:    "sw-button is removed, use mt-button instead. Please review conversion for variant and router-link.",
				Severity:   validation.SeverityWarning,
				Identifier: "sw-button",
				Line:       node.Line,
			})
		}
	})
	return errors
}

func (b ButtonFixer) Supports(v *version.Version) bool {
	// ...existing code...
	return twiglinter.Shopware67Constraint.Check(v)
}

func (b ButtonFixer) Fix(nodes []html.Node) error {
	html.TraverseNode(nodes, func(node *html.ElementNode) {
		if node.Tag == "sw-button" {
			node.Tag = "mt-button"
			var newAttrs html.NodeList
			// Flags to determine additional properties.
			addGhost := false

			for _, attrNode := range node.Attributes {
				// Check if the attribute is an html.Attribute
				if attr, ok := attrNode.(html.Attribute); ok {
					switch attr.Key {
					case "variant":
						lower := strings.ToLower(attr.Value)
						switch lower {
						case "ghost":
							// Remove variant and set ghost.
							addGhost = true
						case "danger":
							// Change value to critical.
							attr.Value = CriticalValue
							newAttrs = append(newAttrs, attr)
						case "ghost-danger":
							// Set critical and also ghost.
							attr.Value = CriticalValue
							newAttrs = append(newAttrs, attr)
							addGhost = true
						case "contrast", "context":
							// Remove attribute
						default:
							newAttrs = append(newAttrs, attr)
						}
					case "router-link":
						// Replace with @click event.
						val := attr.Value
						newAttrs = append(newAttrs, html.Attribute{
							Key:   "@click",
							Value: fmt.Sprintf("this.$router.push('%s')", val),
						})
					default:
						newAttrs = append(newAttrs, attr)
					}
				} else {
					// If it's not an html.Attribute (e.g., TwigIfNode), preserve it as is
					newAttrs = append(newAttrs, attrNode)
				}
			}

			if addGhost {
				newAttrs = append(newAttrs, html.Attribute{
					Key: "ghost",
				})
			}
			node.Attributes = newAttrs
		}
	})
	return nil
}
