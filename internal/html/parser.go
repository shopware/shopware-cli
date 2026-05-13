package html

import "strings"

// Attribute represents an HTML attribute with key and value.
type Attribute struct {
	Key   string
	Value string
}


// Node is the interface for nodes in our AST.
type Node interface {
	Dump(indent int) string
}

type NodeList []Node


// ConfiguredNodeList wraps a NodeList with its associated IndentConfig.
// The Dump method is in format.go.
type ConfiguredNodeList struct {
	Nodes  NodeList
	Config IndentConfig
}

// Dump renders the nodes using the stored configuration

// RawNode holds unchanged text.
type RawNode struct {
	Text string
	Line int // added field
}

// Dump returns the raw text.

// CommentNode represents an HTML comment.
type CommentNode struct {
	Text string
	Line int
}

// Dump returns the comment text with HTML comment syntax.

// TemplateExpressionNode represents a {{...}} template expression.
type TemplateExpressionNode struct {
	Expression string
	Line       int
}

// Dump returns the template expression with {{ }} delimiters.

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
	Line     int // added field
}

// Dump returns the HTML representation of the element and its children.
//

// TwigBlockNode represents a twig block.
type TwigBlockNode struct {
	Name     string
	Children NodeList
	Line     int
}

// Dump returns the twig block with proper formatting.

// TwigIfNode represents a Twig if block.
type TwigIfNode struct {
	Condition        string
	Children         NodeList
	ElseIfConditions []string
	ElseIfChildren   []NodeList
	ElseChildren     NodeList
	Line             int
}

// Dump returns the twig if block with proper formatting
//

// ParentNode represents a twig parent() call.
type ParentNode struct {
	Line int
}


func NewParser(input string) (NodeList, error) {
	p := &parser{source: input}
	return p.parseDocument()
}

func isVoidElement(tag string) bool {
	switch strings.ToLower(tag) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "keygen", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

func TraverseNode(n NodeList, f func(*ElementNode)) {
	for _, node := range n {
		switch node := node.(type) {
		case *ElementNode:
			f(node)
			for _, child := range node.Children {
				TraverseNode(NodeList{child}, f)
			}
		case *TwigBlockNode:
			TraverseNode(node.Children, f)
		case *TemplateExpressionNode:
			// Template expressions don't have children to traverse
			continue
		}
	}
}

// NewParserWithConfig creates a new parser with a specific indentation configuration.
func NewParserWithConfig(input string, config IndentConfig) (NodeList, error) {
	oldConfig := indentConfig
	SetIndentConfig(config)
	defer SetIndentConfig(oldConfig) // Restore original config

	nodes, err := NewParser(input)
	if err != nil {
		return NodeList{}, err
	}

	return nodes, nil
}

// NewAdminParser creates a parser configured for admin twig files (no indentation for twig block children).
func NewAdminParser(input string) (ConfiguredNodeList, error) {
	config := DefaultIndentConfig()
	config.TwigBlockIndentChildren = false

	nodes, err := NewParserWithConfig(input, config)
	if err != nil {
		return ConfiguredNodeList{}, err
	}

	return ConfiguredNodeList{
		Nodes:  nodes,
		Config: config,
	}, nil
}

// NewStorefrontParser creates a parser configured for storefront twig files (indents twig block children).
func NewStorefrontParser(input string) (ConfiguredNodeList, error) {
	config := DefaultIndentConfig()
	config.TwigBlockIndentChildren = true

	nodes, err := NewParserWithConfig(input, config)
	if err != nil {
		return ConfiguredNodeList{}, err
	}

	return ConfiguredNodeList{
		Nodes:  nodes,
		Config: config,
	}, nil
}
