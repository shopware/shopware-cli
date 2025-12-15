package twigparser

import (
	"fmt"
	"strings"
)

// Node represents an AST node.
type Node interface {
	// String provides a debug representation with indentation.
	String(indent string) string
	// Dump outputs the node (and its children) back into source code.
	Dump() string
}

// TextNode holds non‑whitespace plain text.
type TextNode struct {
	Text string
}

func (t *TextNode) String(indent string) string {
	escaped := strings.ReplaceAll(t.Text, "\n", "\\n")
	return fmt.Sprintf("%sTextNode(%q)", indent, escaped)
}

func (t *TextNode) Dump() string {
	return t.Text
}

// WhitespaceNode holds a text fragment that consists solely of
// whitespace (spaces, tabs, newlines, etc).
type WhitespaceNode struct {
	Text string
}

func (w *WhitespaceNode) String(indent string) string {
	escaped := strings.ReplaceAll(w.Text, "\n", "\\n")
	return fmt.Sprintf("%sWhitespaceNode(%q)", indent, escaped)
}

func (w *WhitespaceNode) Dump() string {
	return w.Text
}

// BlockNode represents a Twig block (with opening tag {% block <name> %}
// and ending tag {% endblock %}), and contains nested child nodes.
type BlockNode struct {
	Name     string
	Children NodeList
}

func (b *BlockNode) String(indent string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sBlockNode(Name: %s)\n", indent, b.Name))
	for _, child := range b.Children {
		sb.WriteString(child.String(indent + "  "))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (b *BlockNode) Dump() string {
	var sb strings.Builder
	sb.WriteString("{% block " + b.Name + " %}")
	for _, child := range b.Children {
		sb.WriteString(child.Dump())
	}
	sb.WriteString("{% endblock %}")
	return sb.String()
}

// ParentNode represents the Twig expression {{ parent() }}.
type ParentNode struct{}

func (p *ParentNode) String(indent string) string {
	return fmt.Sprintf("%sParentNode(parent())", indent)
}

func (p *ParentNode) Dump() string {
	return "{{ parent() }}"
}

// SwExtendsNode represents the Twig tag for extending a template.
// It supports both simple and object-literal syntaxes.
type SwExtendsNode struct {
	Template string
	Scopes   []string
}

func (s *SwExtendsNode) String(indent string) string {
	if len(s.Scopes) > 0 {
		return fmt.Sprintf("%sSwExtendsNode(Template: %q, Scopes: %q)", indent, s.Template, s.Scopes)
	}
	return fmt.Sprintf("%sSwExtendsNode(Template: %q)", indent, s.Template)
}

func (s *SwExtendsNode) Dump() string {
	// Dump in canonical form.
	if len(s.Scopes) > 0 {
		// Build a scopes array such as ['default', 'subscription']
		var scopesParts []string
		for _, scope := range s.Scopes {
			scopesParts = append(scopesParts, fmt.Sprintf("'%s'", scope))
		}
		return fmt.Sprintf("{%% sw_extends { template: '%s', scopes: [%s] } %%}",
			s.Template, strings.Join(scopesParts, ", "))
	}
	// Simple syntax?
	return "{% sw_extends '" + s.Template + "' %}"
}

// ForNode represents a for-loop in the template.
type ForNode struct {
	Var        string
	Collection string
	Children   NodeList
}

func (f *ForNode) String(indent string) string {
	// Minimal debug representation.
	s := indent + "ForNode(Var: " + f.Var + ", Collection: " + f.Collection + ")\n"
	for _, child := range f.Children {
		s += child.String(indent+"  ") + "\n"
	}
	return s
}

func (f *ForNode) Dump() string {
	var sb strings.Builder
	sb.WriteString("{% for " + f.Var + " in " + f.Collection + " %}")
	// ...existing code...
	for _, child := range f.Children {
		sb.WriteString(child.Dump())
	}
	sb.WriteString("{% endfor %}")
	return sb.String()
}

// PrintNode represents an expression that prints a variable.
type PrintNode struct {
	Expression string
}

func (p *PrintNode) String(indent string) string {
	return indent + "PrintNode(" + p.Expression + ")"
}

func (p *PrintNode) Dump() string {
	return "{{ " + p.Expression + " }}"
}

// DeprecatedNode represents a deprecated tag in the template.
type DeprecatedNode struct {
	Message string
}

func (d *DeprecatedNode) String(indent string) string {
	return indent + "DeprecatedNode(" + d.Message + ")"
}

func (d *DeprecatedNode) Dump() string {
	return "{% deprecated '" + d.Message + "' %}"
}

// SetNode represents a 'set' assignment in the template.
type SetNode struct {
	Variables []string // left-hand side variable(s)
	Values    []string // right-hand side expression(s) for inline assignment; empty when IsBlock is true
	IsBlock   bool     // true when using block assignment
	Children  NodeList // block assignment content
}

func (s *SetNode) String(indent string) string {
	if s.IsBlock {
		return fmt.Sprintf("%sSetNode(Block, Variables: %v)", indent, s.Variables)
	}
	return fmt.Sprintf("%sSetNode(Inline, Variables: %v, Values: %v)", indent, s.Variables, s.Values)
}

func (s *SetNode) Dump() string {
	if s.IsBlock {
		return "{% set " + joinNames(s.Variables) + " %}" + s.Children.Dump() + "{% endset %}"
	}
	return "{% set " + joinNames(s.Variables) + " = " + joinNames(s.Values) + " %}"
}

// joinNames is a helper to join a slice of strings with ", ".
func joinNames(arr []string) string {
	return strings.Join(arr, ", ")
}

// AutoescapeNode represents an autoescape block in the template.
type AutoescapeNode struct {
	Strategy string   // e.g. "html"
	Children NodeList // content within the autoescape block
}

func (a *AutoescapeNode) String(indent string) string {
	return fmt.Sprintf("%sAutoescapeNode(Strategy: %s)", indent, a.Strategy)
}

func (a *AutoescapeNode) Dump() string {
	return "{% autoescape %}" + a.Children.Dump() + "{% endautoescape %}"
}

// TypesNode represents a types definition tag such as {% types score: 'number' %}
type TypesNode struct {
	Types map[string]string
}

func (t *TypesNode) String(indent string) string {
	return fmt.Sprintf("%sTypesNode(%v)", indent, t.Types)
}

func (t *TypesNode) Dump() string {
	var parts []string
	for key, value := range t.Types {
		parts = append(parts, fmt.Sprintf("%s: %s", key, value))
	}
	return "{% types " + strings.Join(parts, " ") + " %}"
}
