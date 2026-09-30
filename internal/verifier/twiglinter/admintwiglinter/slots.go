package admintwiglinter

import (
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/html"
)

func migratedValueKey(key string) string {
	if strings.HasPrefix(key, ":") {
		return ":model-value"
	}
	return ModelValueAttr
}

func slotName(node *html.ElementNode) string {
	if node.Tag != TemplateTag || len(node.Attributes) != 1 {
		return ""
	}
	attr, ok := node.Attributes[0].(*html.Attribute)
	if !ok || attr.Value != "" {
		return ""
	}
	if strings.HasPrefix(attr.Key, "#") {
		return strings.TrimPrefix(attr.Key, "#")
	}
	if strings.HasPrefix(attr.Key, "v-slot:") {
		return strings.TrimPrefix(attr.Key, "v-slot:")
	}
	return ""
}

// Only plain text or a single Vue interpolation can be represented by a prop.
func slotValue(nodes html.NodeList) (string, bool, bool) {
	var text strings.Builder
	var expression *html.TemplateExpressionNode
	for _, node := range nodes {
		switch n := node.(type) {
		case *html.RawNode:
			text.WriteString(n.Text)
		case *html.TemplateExpressionNode:
			if expression != nil {
				return "", false, false
			}
			expression = n
		default:
			return "", false, false
		}
	}
	if expression != nil {
		if strings.TrimSpace(text.String()) != "" {
			return "", false, false
		}
		return strings.TrimSpace(expression.Expression), true, true
	}
	return strings.TrimSpace(text.String()), false, true
}

func canConvertFieldSlots(node *html.ElementNode, props ...string) bool {
	seen := make(map[string]bool)
	for _, child := range node.Children {
		elem, ok := child.(*html.ElementNode)
		if !ok {
			continue
		}
		name := slotName(elem)
		if !slices.Contains(props, name) {
			if elem.Tag == TemplateTag {
				return false
			}
			continue
		}
		if seen[name] {
			return false
		}
		seen[name] = true
		for _, a := range node.Attributes {
			if attr, ok := a.(*html.Attribute); ok && (attr.Key == name || attr.Key == ":"+name) {
				return false
			}
		}
		if _, _, safe := slotValue(elem.Children); !safe {
			return false
		}
	}
	return true
}

func convertFieldSlots(node *html.ElementNode, props ...string) {
	var children html.NodeList
	for _, child := range node.Children {
		elem, ok := child.(*html.ElementNode)
		if ok && slices.Contains(props, slotName(elem)) {
			value, bound, _ := slotValue(elem.Children)
			key := slotName(elem)
			if bound {
				key = ":" + key
			}
			node.Attributes = append(node.Attributes, &html.Attribute{Key: key, Value: value})
		} else {
			children = append(children, child)
		}
	}
	onlyWhitespace := true
	for _, child := range children {
		raw, ok := child.(*html.RawNode)
		if !ok || strings.TrimSpace(raw.Text) != "" {
			onlyWhitespace = false
			break
		}
	}
	if onlyWhitespace {
		children = nil
	}
	node.Children = children
}
