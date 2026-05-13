package html

import (
	"strings"
)

type AttributeEntityEncodingFromTo struct {
	From string
	To   string
}

var fromTextToEntities = []AttributeEntityEncodingFromTo{
	{From: "\"", To: "&quot;"},
}
var fromEntitiesToText = []AttributeEntityEncodingFromTo{
	{From: "&#34;", To: "\""},
	{From: "&quot;", To: "\""},
	{From: "&#39;", To: "\\"},
}

const htmlCommentStart = "<!--"

// Attribute represents an HTML attribute with key and value.
type Attribute struct {
	Key   string
	Value string
}

func (a Attribute) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()

	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	if a.Value == "" {
		return builder.String() + a.Key
	}

	val := a.Value

	for _, encoding := range fromTextToEntities {
		val = strings.ReplaceAll(val, encoding.From, encoding.To)
	}

	return builder.String() + a.Key + "=\"" + val + "\""
}

// Node is the interface for nodes in our AST.
type Node interface {
	Dump(indent int) string
}

type NodeList []Node

// ConfiguredNodeList wraps a NodeList with its associated IndentConfig
type ConfiguredNodeList struct {
	Nodes  NodeList
	Config IndentConfig
}

// Dump renders the nodes using the stored configuration
func (cnl ConfiguredNodeList) Dump(indent int) string {
	oldConfig := indentConfig
	SetIndentConfig(cnl.Config)
	defer SetIndentConfig(oldConfig)

	return cnl.Nodes.Dump(indent)
}

// IndentConfig holds configuration for indentation in HTML output.
type IndentConfig struct {
	SpaceIndent             bool
	IndentSize              int
	TwigBlockIndentChildren bool
}

// DefaultIndentConfig creates a default indentation config with spaces.
func DefaultIndentConfig() IndentConfig {
	return IndentConfig{
		SpaceIndent:             true,
		IndentSize:              4,
		TwigBlockIndentChildren: true,
	}
}

// GetIndent returns the indentation string based on configuration.
func (c IndentConfig) GetIndent() string {
	if c.SpaceIndent {
		return strings.Repeat(" ", c.IndentSize)
	}
	return "\t"
}

// The global indentation config that will be used by all nodes.
var indentConfig = DefaultIndentConfig()

// SetIndentConfig updates the global indentation configuration.
func SetIndentConfig(config IndentConfig) {
	indentConfig = config
}

func (nodeList NodeList) Dump(indent int) string {
	var builder strings.Builder
	for i, node := range nodeList {
		if _, ok := node.(*CommentNode); ok {
			builder.WriteString(node.Dump(indent))
			builder.WriteString("\n")
			continue
		}
		if i > 0 {
			// Add newline between non-comment nodes if not first
			if _, ok := nodeList[i-1].(*CommentNode); !ok {
				builder.WriteString("\n")

				// Add extra newline between template elements
				if isTemplateElement(node) && i > 0 && isTemplateElement(nodeList[i-1]) {
					builder.WriteString("\n")
				}
			}
		}
		builder.WriteString(node.Dump(indent))
	}

	// Remove trailing newlines
	result := builder.String()
	if len(nodeList) > 0 {
		result = strings.TrimRight(result, "\n")
		// Only add ending newline if the original string had at least one
		if strings.HasSuffix(builder.String(), "\n") {
			result += "\n"
		}
	}

	return result
}

// Helper function to check if a node is a template element.
func isTemplateElement(node Node) bool {
	if elem, ok := node.(*ElementNode); ok {
		return elem.Tag == "template"
	}
	// Also consider twig blocks as template elements for spacing purposes
	if _, ok := node.(*TwigBlockNode); ok {
		return true
	}
	return false
}

// RawNode holds unchanged text.
type RawNode struct {
	Text string
	Line int // added field
}

// Dump returns the raw text.
func (r *RawNode) Dump(indent int) string {
	return r.Text
}

// CommentNode represents an HTML comment.
type CommentNode struct {
	Text string
	Line int
}

// Dump returns the comment text with HTML comment syntax.
func (c *CommentNode) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	builder.WriteString("<!-- " + c.Text + " -->")

	return builder.String()
}

// TemplateExpressionNode represents a {{...}} template expression.
type TemplateExpressionNode struct {
	Expression string
	Line       int
}

// Dump returns the template expression with {{ }} delimiters.
func (t *TemplateExpressionNode) Dump(indent int) string {
	return "{{" + t.Expression + "}}"
}

// ElementNode represents an HTML element.
type ElementNode struct {
	Tag         string
	Attributes  NodeList
	Children    NodeList
	SelfClosing bool
	Line        int // added field
}

// Dump returns the HTML representation of the element and its children.
//
//nolint:gocyclo
func (e *ElementNode) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()

	// Add initial indentation
	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	builder.WriteString("<" + e.Tag)

	attributesDidNewLine := false

	// Add attributes
	if len(e.Attributes) > 0 {
		if len(e.Attributes) == 1 {
			attributeStr := e.Attributes[0].Dump(indent + 1)
			_, isIfNode := e.Attributes[0].(*TwigIfNode)

			if len(attributeStr) > 80 || isIfNode {
				builder.WriteString("\n")
				builder.WriteString(attributeStr)
				builder.WriteString("\n")
				attributesDidNewLine = true
			} else {
				if !isIfNode {
					attributeStr = e.Attributes[0].Dump(0)
				}
				builder.WriteString(" ")
				builder.WriteString(attributeStr)
			}
		} else {
			for _, attr := range e.Attributes {
				builder.WriteString("\n")
				attributesDidNewLine = true
				builder.WriteString(attr.Dump(indent + 1))
			}
			builder.WriteString("\n")
		}
	}

	if attributesDidNewLine {
		for i := 0; i < indent; i++ {
			builder.WriteString(indentStr)
		}
	}

	// Handle self-closing tags
	if e.SelfClosing {
		builder.WriteString("/>")
		return builder.String()
	}

	builder.WriteString(">")

	// Handle children
	if len(e.Children) > 0 {
		// Preserve on p tag the formatting
		if e.Tag == "p" {
			hasLongTemplateExpression := false
			for _, child := range e.Children {
				if tplExpr, ok := child.(*TemplateExpressionNode); ok {
					if len(tplExpr.Dump(0)) > 30 {
						hasLongTemplateExpression = true
						break
					}
				}
			}

			if hasLongTemplateExpression {
				builder.WriteString("\n")
				for _, child := range e.Children {
					if _, ok := child.(*TemplateExpressionNode); ok {
						for j := 0; j < indent+1; j++ {
							builder.WriteString(indentStr)
						}
						builder.WriteString(child.Dump(indent+1) + "\n")
					} else if raw, ok := child.(*RawNode); ok {
						trimmed := strings.TrimSpace(raw.Text)
						if trimmed != "" {
							for j := 0; j < indent+1; j++ {
								builder.WriteString(indentStr)
							}
							builder.WriteString(trimmed + "\n")
						}
					} else {
						builder.WriteString(child.Dump(indent + 1))
					}
				}
				for i := 0; i < indent; i++ {
					builder.WriteString(indentStr)
				}
			} else {
				for _, child := range e.Children {
					builder.WriteString(child.Dump(indent))
				}
			}
		} else {
			// Special case: if all children are text/comments/template expressions, keep them on same line
			allSimpleNodes := true
			hasLongTemplateExpression := false
			multipleTemplateExpressions := 0
			multipleShortTemplateExpressions := false

			// Count template expressions and check for long ones
			for _, child := range e.Children {
				if tplExpr, ok := child.(*TemplateExpressionNode); ok {
					multipleTemplateExpressions++
					if len(tplExpr.Dump(0)) > 30 {
						hasLongTemplateExpression = true
					}
				} else if _, ok := child.(*RawNode); !ok {
					if _, ok := child.(*CommentNode); !ok {
						allSimpleNodes = false
						break
					}
				}
			}

			// Special case: if we have a single RawNode child with structured content,
			// treat it as complex content that needs proper indentation
			if allSimpleNodes && len(e.Children) == 1 {
				if rawChild, ok := e.Children[0].(*RawNode); ok {
					if strings.Contains(rawChild.Text, "\n") {
						// Check if the content has meaningful indentation structure
						lines := strings.Split(rawChild.Text, "\n")
						hasIndentedContent := false
						for _, line := range lines {
							trimmed := strings.TrimLeft(line, " \t")
							if trimmed != "" && len(line) > len(trimmed) {
								hasIndentedContent = true
								break
							}
						}
						if hasIndentedContent {
							allSimpleNodes = false
						}
					}
				}
			}

			// Check if we have multiple short template expressions
			if multipleTemplateExpressions > 1 && !hasLongTemplateExpression {
				// Check if they're short enough to stay on one line
				totalLength := 0
				for _, child := range e.Children {
					if tplExpr, ok := child.(*TemplateExpressionNode); ok {
						totalLength += len(tplExpr.Dump(indent + 1))
					}
				}
				// If the combined length is short, keep them on the same line
				if totalLength <= 100 {
					multipleShortTemplateExpressions = true
				}
			}

			if allSimpleNodes {
				// Format based on content
				if hasLongTemplateExpression || (multipleTemplateExpressions > 1 && !multipleShortTemplateExpressions) {
					// For template expressions that are long or multiple long ones, add nice formatting
					builder.WriteString("\n")
					for _, child := range e.Children {
						if _, ok := child.(*TemplateExpressionNode); ok {
							for j := 0; j < indent+1; j++ {
								builder.WriteString(indentStr)
							}
							builder.WriteString(child.Dump(indent+1) + "\n")
						} else if raw, ok := child.(*RawNode); ok {
							trimmed := strings.TrimSpace(raw.Text)
							if trimmed != "" {
								for j := 0; j < indent+1; j++ {
									builder.WriteString(indentStr)
								}
								builder.WriteString(trimmed + "\n")
							}
						} else {
							builder.WriteString(child.Dump(indent + 1))
						}
					}
					for i := 0; i < indent; i++ {
						builder.WriteString(indentStr)
					}
				} else {
					// For simple content, keep on the same line
					for _, child := range e.Children {
						builder.WriteString(child.Dump(indent))
					}
				}
			} else {
				// For complex nodes, format with proper indentation
				var nonEmptyChildren NodeList
				for _, child := range e.Children {
					if raw, ok := child.(*RawNode); ok {
						if strings.TrimSpace(raw.Text) != "" {
							nonEmptyChildren = append(nonEmptyChildren, raw)
						}
					} else {
						nonEmptyChildren = append(nonEmptyChildren, child)
					}
				}

				// Check for template elements and add extra newlines between them
				for i, child := range nonEmptyChildren {
					builder.WriteString("\n")

					// Add an extra newline between template elements
					if i > 0 && isTemplateElement(child) && isTemplateElement(nonEmptyChildren[i-1]) {
						builder.WriteString("\n")
					}

					if elementChild, ok := child.(*ElementNode); ok {
						builder.WriteString(elementChild.Dump(indent + 1))
					} else if twigBlockChild, ok := child.(*TwigBlockNode); ok {
						builder.WriteString(twigBlockChild.Dump(indent + 1))
					} else if rawChild, ok := child.(*RawNode); ok {
						// Special handling for RawNode with newlines
						if strings.Contains(rawChild.Text, "\n") {
							// Re-indent multi-line raw content
							lines := strings.Split(rawChild.Text, "\n")
							var contentLines []string

							// Extract non-empty content lines
							for _, line := range lines {
								trimmed := strings.TrimLeft(line, " \t")
								if trimmed != "" {
									contentLines = append(contentLines, trimmed)
								}
							}

							// Output content lines with proper indentation
							for idx, trimmed := range contentLines {
								for j := 0; j < indent+1; j++ {
									builder.WriteString(indentStr)
								}
								builder.WriteString(trimmed)
								if idx < len(contentLines)-1 {
									builder.WriteString("\n")
								}
							}
						} else {
							// Single line content, use original logic
							for j := 0; j < indent+1; j++ {
								builder.WriteString(indentStr)
							}
							builder.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
						}
					} else {
						for j := 0; j < indent+1; j++ {
							builder.WriteString(indentStr)
						}
						builder.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
					}
				}
				builder.WriteString("\n")
				for i := 0; i < indent; i++ {
					builder.WriteString(indentStr)
				}
			}
		}
	}

	builder.WriteString("</" + e.Tag + ">")
	return builder.String()
}

// TwigBlockNode represents a twig block.
type TwigBlockNode struct {
	Name     string
	Children NodeList
	Line     int
}

// Dump returns the twig block with proper formatting.
func (t *TwigBlockNode) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}
	builder.WriteString("{% block " + t.Name + " %}")

	// Filter out empty nodes and normalize newlines
	var nonEmptyChildren NodeList
	for _, child := range t.Children {
		if raw, ok := child.(*RawNode); ok {
			if strings.TrimSpace(raw.Text) != "" {
				nonEmptyChildren = append(nonEmptyChildren, raw)
			}
		} else if twigBlock, ok := child.(*TwigBlockNode); ok {
			if strings.TrimSpace(twigBlock.Dump(0)) != "" {
				nonEmptyChildren = append(nonEmptyChildren, twigBlock)
			}
		} else {
			nonEmptyChildren = append(nonEmptyChildren, child)
		}
	}

	if len(nonEmptyChildren) > 0 {
		builder.WriteString("\n")
		childIndent := indent
		if indentConfig.TwigBlockIndentChildren {
			childIndent = indent + 1
		}

		for i, child := range nonEmptyChildren {
			if elementChild, ok := child.(*ElementNode); ok {
				builder.WriteString(elementChild.Dump(childIndent))
			} else if tplChild, ok := child.(*TemplateExpressionNode); ok {
				// Template expressions need proper indentation when they're direct children of twig blocks
				for j := 0; j < childIndent; j++ {
					builder.WriteString(indentStr)
				}
				builder.WriteString(tplChild.Dump(childIndent))
			} else {
				builder.WriteString(child.Dump(childIndent))
			}

			_, isComment := child.(*CommentNode)

			if i < len(nonEmptyChildren)-1 {
				// Add an extra newline between elements
				if isComment {
					builder.WriteString("\n")
				} else {
					builder.WriteString("\n\n")
				}
			}
		}
		builder.WriteString("\n")

		for i := 0; i < indent; i++ {
			builder.WriteString(indentStr)
		}

		builder.WriteString("{% endblock %}")
	} else {
		builder.WriteString("{% endblock %}")
	}

	return builder.String()
}

// TwigIfNode represents a Twig if block
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
//nolint:gocyclo
func (t *TwigIfNode) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()

	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	builder.WriteString("{% if " + t.Condition + " %}")

	// Filter out empty nodes and normalize newlines for if branch
	var nonEmptyChildren NodeList
	for _, child := range t.Children {
		if raw, ok := child.(*RawNode); ok {
			if strings.TrimSpace(raw.Text) != "" {
				nonEmptyChildren = append(nonEmptyChildren, raw)
			}
		} else {
			nonEmptyChildren = append(nonEmptyChildren, child)
		}
	}

	if len(nonEmptyChildren) > 0 {
		builder.WriteString("\n")
		for i, child := range nonEmptyChildren {
			if elementChild, ok := child.(*ElementNode); ok {
				builder.WriteString(elementChild.Dump(indent + 1))
			} else {
				for i := 0; i < indent+1; i++ {
					builder.WriteString(indentStr)
				}
				builder.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
			}
			if i < len(nonEmptyChildren)-1 {
				// Add an extra newline between elements
				builder.WriteString("\n")
			}
		}
		builder.WriteString("\n")
	}

	// Handle elseif branches if they exist
	for i, condition := range t.ElseIfConditions {
		for i := 0; i < indent; i++ {
			builder.WriteString(indentStr)
		}
		builder.WriteString("{% elseif " + condition + " %}")

		// Filter out empty nodes and normalize newlines for elseif branch
		nonEmptyChildren = NodeList{}
		for _, child := range t.ElseIfChildren[i] {
			if raw, ok := child.(*RawNode); ok {
				if strings.TrimSpace(raw.Text) != "" {
					nonEmptyChildren = append(nonEmptyChildren, raw)
				}
			} else {
				nonEmptyChildren = append(nonEmptyChildren, child)
			}
		}

		if len(nonEmptyChildren) > 0 {
			builder.WriteString("\n")
			for j, child := range nonEmptyChildren {
				if elementChild, ok := child.(*ElementNode); ok {
					builder.WriteString(elementChild.Dump(indent + 1))
				} else {
					for i := 0; i < indent+1; i++ {
						builder.WriteString(indentStr)
					}
					builder.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
				}
				if j < len(nonEmptyChildren)-1 {
					// Add an extra newline between elements
					builder.WriteString("\n")
				}
			}
			builder.WriteString("\n")
		}
	}

	// Handle else branch if it exists
	if len(t.ElseChildren) > 0 {
		for i := 0; i < indent; i++ {
			builder.WriteString(indentStr)
		}
		builder.WriteString("{% else %}")

		// Filter out empty nodes and normalize newlines for else branch
		var nonEmptyElseChildren NodeList
		for _, child := range t.ElseChildren {
			if raw, ok := child.(*RawNode); ok {
				if strings.TrimSpace(raw.Text) != "" {
					nonEmptyElseChildren = append(nonEmptyElseChildren, raw)
				}
			} else {
				nonEmptyElseChildren = append(nonEmptyElseChildren, child)
			}
		}

		if len(nonEmptyElseChildren) > 0 {
			builder.WriteString("\n")
			for i, child := range nonEmptyElseChildren {
				if elementChild, ok := child.(*ElementNode); ok {
					builder.WriteString(elementChild.Dump(indent + 1))
				} else {
					for i := 0; i < indent+1; i++ {
						builder.WriteString(indentStr)
					}
					builder.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
				}
				if i < len(nonEmptyElseChildren)-1 {
					// Add an extra newline between elements
					builder.WriteString("\n")
				}
			}
			builder.WriteString("\n")
		}
	}

	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	builder.WriteString("{% endif %}")
	return builder.String()
}

// ParentNode represents a twig parent() call
type ParentNode struct {
	Line int
}

func (p *ParentNode) Dump(indent int) string {
	var builder strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		builder.WriteString(indentStr)
	}

	builder.WriteString("{% parent() %}")

	return builder.String()
}

// NewParser creates a new parser for the given input.
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
