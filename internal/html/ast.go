// AST node types for the html package. Each node has a Dump(int) string
// method (defined in format.go) that renders it back to its textual form.
package html

// Attribute represents an HTML attribute with key and value.
type Attribute struct {
	Key   string
	Value string
}

// Node is the interface for nodes in the AST.
type Node interface {
	Dump(indent int) string
}

// NodeList is a sequence of AST nodes. It has its own Dump method that
// arranges children with appropriate inter-node whitespace.
type NodeList []Node

// TwigTrim records the whitespace-control modifiers on a single Twig
// delimiter pair. `{%- if -%}` is TwigTrim{Left: true, Right: true} on
// the if-header, for example. Twig strips surrounding whitespace at
// render time when a side is true, so the formatter must emit `{%-`/`-%}`
// (or `{{-`/`-}}`, `{#-`/`-#}`) verbatim — losing them would change
// rendered output.
type TwigTrim struct {
	Left  bool // open delimiter has a leading '-' (e.g. {%-)
	Right bool // close delimiter has a trailing '-' (e.g. -%})
}

// ConfiguredNodeList wraps a NodeList with the IndentConfig under which it
// should format. NewAdminParser and NewStorefrontParser return this so the
// caller can Dump(0) without managing config separately.
type ConfiguredNodeList struct {
	Nodes  NodeList
	Config IndentConfig
}

// RawNode holds verbatim source text — anything outside structured HTML or
// Twig syntax, plus the bytes of any Twig tag that has no registered handler.
type RawNode struct {
	Text string
	Line int
}

// CommentNode represents an HTML <!-- ... --> comment.
type CommentNode struct {
	Text string
	Line int
}

// TemplateExpressionNode represents a `{{ ... }}` Twig expression.
type TemplateExpressionNode struct {
	Expression string
	Trim       TwigTrim
	Line       int
}

// ElementNode represents an HTML element.
type ElementNode struct {
	Tag         string
	Attributes  NodeList
	Children    NodeList
	SelfClosing bool
	// Unclosed reports that the element opened with `<tag>` but its
	// children parser yielded on an outer Twig terminator (e.g.
	// `{% endblock %}`) before reaching `</tag>`. The closing tag is
	// elsewhere in the source, typically wrapped in another control-flow
	// block. The formatter therefore does NOT emit `</tag>` for unclosed
	// elements — the matching `</tag>` lives as a RawNode further down.
	Unclosed bool
	Line     int
}

// TwigBlockNode represents `{% block name %}...{% endblock %}`.
// OpenTrim is the trim flags on the `{% block name %}` delimiters; CloseTrim
// is the trim flags on the `{% endblock %}` delimiters.
type TwigBlockNode struct {
	Name      string
	Children  NodeList
	OpenTrim  TwigTrim
	CloseTrim TwigTrim
	Line      int
}

// TwigIfBranch is one conditional branch of a {% if %}...{% endif %} block.
// The first branch in TwigIfNode.Branches is the "if" itself; subsequent
// entries are "elseif" branches. The else (no-condition) branch is held
// separately on TwigIfNode.ElseChildren. Trim is the trim flags on the
// branch's own header delimiters.
type TwigIfBranch struct {
	Condition string
	Body      NodeList
	Trim      TwigTrim
}

// TwigIfNode represents `{% if %}...{% elseif %}...{% else %}...{% endif %}`.
// Branches[0] is always the "if"; Branches[1..] are "elseif"s.
// ElseChildren is nil/empty when there is no {% else %} clause.
type TwigIfNode struct {
	Branches     []TwigIfBranch
	ElseChildren NodeList
	// ElseTrim is the trim flags on the `{% else %}` delimiters when the
	// else clause is present.
	ElseTrim TwigTrim
	// EndTrim is the trim flags on the `{% endif %}` delimiters.
	EndTrim TwigTrim
	Line    int
}

// ParentNode represents `{% parent %}` or `{% parent() %}`.
type ParentNode struct {
	Trim TwigTrim
	Line int
}
