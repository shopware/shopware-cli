package html

import (
	"fmt"
	"strings"
	"unicode"
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

// IndentConfig holds configuration for indentation in HTML output.
type IndentConfig struct {
	SpaceIndent bool
	IndentSize  int
}

// DefaultIndentConfig creates a default indentation config with spaces.
func DefaultIndentConfig() IndentConfig {
	return IndentConfig{
		SpaceIndent: true,
		IndentSize:  4,
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
		for i, child := range nonEmptyChildren {
			if elementChild, ok := child.(*ElementNode); ok {
				builder.WriteString(elementChild.Dump(indent + 1))
			} else {
				builder.WriteString(child.Dump(indent + 1))
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

// Parser holds the state for our simple parser.
type Parser struct {
	input  string
	pos    int
	length int
}

// NewParser creates a new parser for the given input.
func NewParser(input string) (NodeList, error) {
	p := &Parser{input: input, pos: 0, length: len(input)}

	return p.parseNodes("")
}

// current returns the current byte (or zero if at the end).
func (p *Parser) current() byte {
	if p.pos >= p.length {
		return 0
	}
	return p.input[p.pos]
}

// peek returns the next n characters (or what remains).
func (p *Parser) peek(n int) string {
	if p.pos+n > p.length {
		return p.input[p.pos:]
	}
	return p.input[p.pos : p.pos+n]
}

// skipWhitespace advances the position over any whitespace.
func (p *Parser) skipWhitespace() {
	for p.pos < p.length &&
		(p.input[p.pos] == ' ' || p.input[p.pos] == '\n' ||
			p.input[p.pos] == '\r' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

// Helper to get line number at a given position.
func (p *Parser) getLineAt(pos int) int {
	return strings.Count(p.input[:pos], "\n") + 1
}

// parseComment parses an HTML comment and returns a CommentNode
func (p *Parser) parseComment() (*CommentNode, error) {
	if p.peek(4) != htmlCommentStart {
		//nolint: nilnil
		return nil, nil
	}
	startPos := p.pos
	p.pos += 4 // skip "<!--"

	start := p.pos
	idx := strings.Index(p.input[p.pos:], "-->")
	if idx == -1 {
		return nil, fmt.Errorf("unterminated comment starting at pos %d", startPos)
	}

	commentText := strings.TrimSpace(p.input[start : start+idx])
	p.pos += idx + 3 // skip past "-->"

	return &CommentNode{
		Text: commentText,
		Line: p.getLineAt(startPos),
	}, nil
}

// parseNodes parses a list of nodes until an optional stop tag (used for element children).
//
//nolint:gocyclo
func (p *Parser) parseNodes(stopTag string) (NodeList, error) {
	var nodes NodeList
	rawStart := p.pos

	for p.pos < p.length {
		// Check for endblock if we're parsing twig block children
		if stopTag == "" && p.peek(2) == "{%" {
			peek := p.input[p.pos:]
			if strings.HasPrefix(peek, "{% endblock") {
				break
			}
		}

		if p.peek(2) == "{%" {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if strings.TrimSpace(text) != "" {
					nodes = append(nodes, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}

			// Try parsing twig directives first
			directive, err := p.parseTwigDirective()
			if err != nil {
				return nodes, err
			}
			if directive != nil {
				nodes = append(nodes, directive)
				rawStart = p.pos
				continue
			}

			// If not a directive, try parsing as a block
			startPos := p.pos
			block, err := p.parseTwigBlock()
			if err != nil {
				return nodes, err
			}
			if block != nil {
				nodes = append(nodes, block)
				rawStart = p.pos
				continue
			}

			// If not a block, try parsing as an if statement
			p.pos = startPos
			ifNode, err := p.parseTwigIf()
			if err != nil {
				return nodes, err
			}
			if ifNode != nil {
				nodes = append(nodes, ifNode)
				rawStart = p.pos
				continue
			}

			// If it wasn't a block or if statement, reset position and continue as raw text
			p.pos = startPos
		}

		// Parse template expressions {{ ... }}
		if p.peek(2) == "{{" {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if text != "" {
					nodes = append(nodes, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}

			expression, err := p.parseTemplateExpression()
			if err != nil {
				return nodes, err
			}

			nodes = append(nodes, expression)
			rawStart = p.pos
			continue
		}

		if p.peek(4) == htmlCommentStart {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if strings.TrimSpace(text) != "" {
					nodes = append(nodes, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}
			comment, err := p.parseComment()
			if err != nil {
				return nodes, err
			}
			nodes = append(nodes, comment)
			rawStart = p.pos
			continue
		}

		// If we're about to hit a closing tag for the current element, break.
		if p.current() == '<' && p.peek(2) == "</" {
			savedPos := p.pos
			p.pos += 2
			p.skipWhitespace()
			closingTag := p.parseTagName()
			p.pos = savedPos
			if stopTag != "" && closingTag == stopTag {
				break
			}
		}

		if p.current() == '<' && p.peek(2) != htmlCommentStart {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if strings.TrimSpace(text) != "" {
					nodes = append(nodes, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}
			element, err := p.parseElement()
			if err != nil {
				return nodes, err
			}
			nodes = append(nodes, element)
			rawStart = p.pos
		} else {
			p.pos++
		}
	}

	if rawStart < p.pos {
		text := p.input[rawStart:p.pos]
		if strings.TrimSpace(text) != "" {
			nodes = append(nodes, &RawNode{
				Text: text,
				Line: p.getLineAt(rawStart),
			})
		}
	}
	return nodes, nil
}

// isVoidElement returns true if the tag is a void element (e.g., <br> does not require a closing tag)
func isVoidElement(tag string) bool {
	switch strings.ToLower(tag) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "keygen", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// parseElement parses an HTML element starting at the current position (assumes a '<').
func (p *Parser) parseElement() (Node, error) {
	// Record start position for line number.
	startPos := p.pos
	if p.current() != '<' {
		//nolint: nilnil
		return nil, nil
	}
	p.pos++ // skip '<'
	p.skipWhitespace()

	tagName := p.parseTagName()
	if tagName == "" {
		return nil, fmt.Errorf("empty tag name at pos %d", p.pos)
	}

	node := &ElementNode{
		Tag:        tagName,
		Attributes: NodeList{},
		Children:   NodeList{},
		Line:       p.getLineAt(startPos),
	}

	// Parse element attributes.
	for p.pos < p.length {
		p.skipWhitespace()
		// Check for Twig directives within attributes
		if p.peek(2) == "{%" {
			ifNode, err := p.parseTwigIf()
			if err != nil {
				return nil, err
			}
			if ifNode != nil {
				node.Attributes = append(node.Attributes, ifNode)
				// After parsing a Twig directive, we need to skip whitespace again
				p.skipWhitespace()
				continue
			}
		}

		if p.current() == '>' || (p.current() == '/' && p.peek(2) == "/>") {
			break
		}
		attrName := p.parseAttrName()
		if attrName == "" {
			break
		}
		p.skipWhitespace()
		var attrVal string
		if p.current() == '=' {
			p.pos++ // skip '='
			p.skipWhitespace()
			attrVal = p.parseAttrValue()
		}
		// Append attribute preserving order.
		node.Attributes = append(node.Attributes, Attribute{Key: attrName, Value: attrVal})
	}

	// Check for self-closing tag.
	if p.current() == '/' {
		p.pos++ // skip '/'
		if p.current() != '>' {
			return nil, fmt.Errorf("expected '>' after '/' at pos %d", p.pos)
		}
		p.pos++ // skip '>'
		node.SelfClosing = true
		return node, nil
	}
	if p.current() == '>' {
		p.pos++ // skip '>'
		if isVoidElement(tagName) {
			node.SelfClosing = true
			return node, nil
		}
	} else {
		// Add more context to the error message
		surroundingText := ""
		start := p.pos - 10
		if start < 0 {
			start = 0
		}
		end := p.pos + 10
		if end > p.length {
			end = p.length
		}
		surroundingText = p.input[start:end]
		return nil, fmt.Errorf("expected '>' at pos %d, surrounding text: '%s', current byte: '%c'", p.pos, surroundingText, p.current())
	}

	// Parse children until the corresponding closing tag.
	children, err := p.parseElementChildren(node.Tag)
	if err != nil {
		return nil, err
	}
	node.Children = children

	return node, nil
}

// parseElementChildren parses the child nodes of an element until the closing tag is reached.
func (p *Parser) parseElementChildren(tag string) (NodeList, error) {
	var children NodeList
	rawStart := p.pos

	for p.pos < p.length {
		if p.peek(4) == htmlCommentStart {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if text != "" {
					children = append(children, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}
			comment, err := p.parseComment()
			if err != nil {
				return children, err
			}
			children = append(children, comment)
			rawStart = p.pos
			continue
		}

		// Parse template expressions {{ ... }}
		if p.peek(2) == "{{" {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if text != "" {
					children = append(children, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}

			expression, err := p.parseTemplateExpression()
			if err != nil {
				return children, err
			}

			children = append(children, expression)
			rawStart = p.pos
			continue
		}

		// Check for a closing tag.
		if p.current() == '<' && p.peek(2) == "</" {
			savedPos := p.pos
			p.pos += 2 // skip "</"
			p.skipWhitespace()
			closingTag := p.parseTagName()
			p.skipWhitespace()
			if p.current() == '>' {
				p.pos++ // skip '>'
			} else {
				return children,
					fmt.Errorf("expected '>' for closing tag at pos %d", p.pos)
			}
			if closingTag == tag {
				// Add any raw text before the closing tag.
				if rawStart < savedPos {
					text := p.input[rawStart:savedPos]
					if text != "" {
						children = append(children, &RawNode{
							Text: text,
							Line: p.getLineAt(rawStart),
						})
					}
				}
				return children, nil
			} else {
				// Not the matching closing tag; reset and continue.
				p.pos = savedPos
			}
		}

		if p.current() == '<' && p.peek(2) != htmlCommentStart {
			if p.pos > rawStart {
				text := p.input[rawStart:p.pos]
				if text != "" {
					children = append(children, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
			}
			child, err := p.parseElement()
			if err != nil {
				return children, err
			}
			children = append(children, child)
			rawStart = p.pos
		} else {
			p.pos++
		}
	}
	return children, nil
}

// parseTagName parses a tag or attribute name (letters, digits, '-' and ':').
func (p *Parser) parseTagName() string {
	start := p.pos
	for p.pos < p.length {
		c := p.input[p.pos]
		if unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)) || c == '-' || c == ':' {
			p.pos++
		} else {
			break
		}
	}
	return p.input[start:p.pos]
}

// parseAttrName parses an attribute name.
func (p *Parser) parseAttrName() string {
	start := p.pos
	// Accept characters until whitespace, '=', '>', or '/'
	for p.pos < p.length {
		c := p.input[p.pos]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' ||
			c == '=' || c == '>' || c == '/' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

// parseAttrValue parses an attribute value (expects a quoted string).
func (p *Parser) parseAttrValue() string {
	if p.current() == '"' {
		p.pos++ // skip opening "
		start := p.pos
		// Continue until we find a closing quote or reach the end
		for p.pos < p.length && p.current() != '"' {
			p.pos++
		}

		val := p.input[start:p.pos]

		if p.pos < p.length && p.current() == '"' {
			p.pos++ // skip closing "
		}

		for _, encoding := range fromEntitiesToText {
			val = strings.ReplaceAll(val, encoding.From, encoding.To)
		}

		return val
	}
	// Allow unquoted values.
	start := p.pos
	for p.pos < p.length &&
		p.current() != ' ' && p.current() != '>' && p.current() != '\n' && p.current() != '\r' {
		p.pos++
	}

	val := p.input[start:p.pos]

	for _, encoding := range fromEntitiesToText {
		val = strings.ReplaceAll(val, encoding.From, encoding.To)
	}

	return val
}

func (p *Parser) parseTwigDirective() (Node, error) {
	if p.peek(2) != "{%" {
		//nolint: nilnil
		return nil, nil
	}

	startPos := p.pos
	p.pos += 2 // skip "{%"
	p.skipWhitespace()

	// Check if it's a parent() call
	if strings.HasPrefix(p.input[p.pos:], "parent()") {
		p.pos += 8 // skip "parent()"
		p.skipWhitespace()
		if p.peek(2) != "%}" {
			return nil, fmt.Errorf("unclosed parent directive at pos %d", startPos)
		}
		p.pos += 2 // skip "%}"
		return &ParentNode{Line: p.getLineAt(startPos)}, nil
	}

	// Handle {% parent %} directive (without parentheses)
	if strings.HasPrefix(p.input[p.pos:], "parent") {
		p.pos += 6 // skip "parent"
		p.skipWhitespace()
		if p.peek(2) != "%}" {
			return nil, fmt.Errorf("unclosed parent directive at pos %d", startPos)
		}
		p.pos += 2 // skip "%}"
		return &ParentNode{Line: p.getLineAt(startPos)}, nil
	}

	// Reset position if it's not a recognized directive
	p.pos = startPos
	//nolint: nilnil
	return nil, nil
}

func (p *Parser) parseTwigBlock() (Node, error) {
	if p.peek(2) != "{%" {
		//nolint: nilnil
		return nil, nil
	}

	startPos := p.pos
	p.pos += 2 // skip "{%"
	p.skipWhitespace()

	// Check if it's a block
	if !strings.HasPrefix(p.input[p.pos:], "block") {
		p.pos = startPos
		//nolint: nilnil
		return nil, nil
	}
	p.pos += 5 // skip "block"
	p.skipWhitespace()

	// Parse block name
	start := p.pos
	for p.pos < p.length && p.current() != '%' && p.current() != ' ' {
		p.pos++
	}
	name := strings.TrimSpace(p.input[start:p.pos])

	// Skip to end of opening tag
	for p.pos < p.length && p.peek(2) != "%}" {
		p.pos++
	}
	if p.peek(2) != "%}" {
		return nil, fmt.Errorf("unclosed block tag at pos %d", startPos)
	}
	p.pos += 2 // skip "%}"

	// Parse children until endblock
	children, err := p.parseNodes("")
	if err != nil {
		return nil, err
	}

	// Look for endblock
	p.skipWhitespace()
	if !strings.HasPrefix(p.input[p.pos:], "{%") {
		return nil, fmt.Errorf("missing endblock at pos %d", p.pos)
	}
	p.pos += 2 // skip "{%"
	p.skipWhitespace()

	if !strings.HasPrefix(p.input[p.pos:], "endblock") {
		return nil, fmt.Errorf("missing endblock at pos %d", p.pos)
	}
	p.pos += 8 // skip "endblock"

	// Skip to end of closing tag
	for p.pos < p.length && p.peek(2) != "%}" {
		p.pos++
	}
	if p.peek(2) != "%}" {
		return nil, fmt.Errorf("unclosed endblock tag at pos %d", p.pos)
	}
	p.pos += 2 // skip "%}"

	return &TwigBlockNode{
		Name:     name,
		Children: children,
		Line:     p.getLineAt(startPos),
	}, nil
}

// parseTwigIf parses a {% if ... %} ... {% endif %} block and returns a TwigIfNode
func (p *Parser) parseTwigIf() (Node, error) {
	if p.peek(2) != "{%" {
		//nolint: nilnil
		return nil, nil
	}

	startPos := p.pos
	p.pos += 2 // skip "{%"
	p.skipWhitespace()

	// Check if it's an if statement
	if !strings.HasPrefix(p.input[p.pos:], "if") {
		p.pos = startPos
		//nolint: nilnil
		return nil, nil
	}
	p.pos += 2 // skip "if"
	p.skipWhitespace()

	// Parse condition
	start := p.pos
	for p.pos < p.length && p.peek(2) != "%}" {
		p.pos++
	}
	condition := strings.TrimSpace(p.input[start:p.pos])

	if p.peek(2) != "%}" {
		return nil, fmt.Errorf("unclosed if tag at pos %d", startPos)
	}
	p.pos += 2 // skip "%}"

	// Parse the if branch
	ifChildren, err := p.parseIfBranch()
	if err != nil {
		return nil, err
	}

	// Initialize elseif condition and children slices
	var elseIfConditions []string
	var elseIfChildren []NodeList

	// Parse any elseif branches
	for {
		// Check if we've reached an elseif
		if p.peek(2) == "{%" && strings.HasPrefix(p.input[p.pos+2:], " elseif") {
			p.pos += 2 // skip "{%"
			p.skipWhitespace()
			p.pos += 6 // skip "elseif"
			p.skipWhitespace()

			// Parse elseif condition
			start := p.pos
			for p.pos < p.length && p.peek(2) != "%}" {
				p.pos++
			}
			elseifCondition := strings.TrimSpace(p.input[start:p.pos])

			if p.peek(2) != "%}" {
				return nil, fmt.Errorf("unclosed elseif tag at pos %d", p.pos)
			}
			p.pos += 2 // skip "%}"

			// Parse elseif branch
			elseifBranch, err := p.parseIfBranch()
			if err != nil {
				return nil, err
			}

			// Add to slices
			elseIfConditions = append(elseIfConditions, elseifCondition)
			elseIfChildren = append(elseIfChildren, elseifBranch)
		} else {
			break
		}
	}

	// Parse the else branch if it exists
	var elseChildren NodeList
	if p.peek(2) == "{%" && strings.HasPrefix(p.input[p.pos+2:], " else") {
		p.pos += 2 // skip "{%"
		p.skipWhitespace()
		p.pos += 4 // skip "else"
		p.skipWhitespace()

		// Skip to the end of the else tag
		for p.pos < p.length && p.peek(2) != "%}" {
			p.pos++
		}
		if p.peek(2) != "%}" {
			return nil, fmt.Errorf("unclosed else tag at pos %d", p.pos)
		}
		p.pos += 2 // skip "%}"

		// Parse else branch
		elseChildren, err = p.parseIfBranch()
		if err != nil {
			return nil, err
		}
	}

	// Look for endif
	if p.peek(2) != "{%" {
		return nil, fmt.Errorf("missing endif at pos %d", p.pos)
	}
	p.pos += 2 // skip "{%"
	p.skipWhitespace()

	if !strings.HasPrefix(p.input[p.pos:], "endif") {
		return nil, fmt.Errorf("missing endif at pos %d", p.pos)
	}
	p.pos += 5 // skip "endif"

	// Skip to end of closing tag
	for p.pos < p.length && p.peek(2) != "%}" {
		p.pos++
	}
	if p.peek(2) != "%}" {
		return nil, fmt.Errorf("unclosed endif tag at pos %d", p.pos)
	}
	p.pos += 2 // skip "%}"

	return &TwigIfNode{
		Condition:        condition,
		Children:         ifChildren,
		ElseIfConditions: elseIfConditions,
		ElseIfChildren:   elseIfChildren,
		ElseChildren:     elseChildren,
		Line:             p.getLineAt(startPos),
	}, nil
}

// parseIfBranch parses the contents of an if or else branch until it encounters
// an {% else %}, {% elseif %} or {% endif %} tag
func (p *Parser) parseIfBranch() (NodeList, error) {
	var nodes NodeList
	rawStart := p.pos

	for p.pos < p.length {
		// Check for else, elseif or endif
		if p.peek(2) == "{%" {
			nextTag := p.input[p.pos+2:]
			if strings.HasPrefix(strings.TrimSpace(nextTag), "else") ||
				strings.HasPrefix(strings.TrimSpace(nextTag), "elseif") ||
				strings.HasPrefix(strings.TrimSpace(nextTag), "endif") {
				break
			}
		}

		// Handle raw text
		if p.pos > rawStart {
			if p.peek(2) == "{%" || p.peek(2) == "{{" || p.peek(4) == htmlCommentStart || p.current() == '<' {
				text := p.input[rawStart:p.pos]
				if text != "" {
					nodes = append(nodes, &RawNode{
						Text: text,
						Line: p.getLineAt(rawStart),
					})
				}
				rawStart = p.pos
			}
		}

		// Try parsing twig directives first
		directive, err := p.parseTwigDirective()
		if err != nil {
			return nodes, err
		}
		if directive != nil {
			nodes = append(nodes, directive)
			rawStart = p.pos
			continue
		}

		// If not a directive, try parsing as a block
		block, err := p.parseTwigBlock()
		if err != nil {
			return nodes, err
		}
		if block != nil {
			nodes = append(nodes, block)
			rawStart = p.pos
			continue
		}

		// Try parsing template expressions {{ ... }}
		expr, err := p.parseTemplateExpression()
		if err != nil {
			return nodes, err
		}
		if expr != nil {
			nodes = append(nodes, expr)
			rawStart = p.pos
			continue
		}

		// Try parsing HTML comments
		comment, err := p.parseComment()
		if err != nil {
			return nodes, err
		}
		if comment != nil {
			nodes = append(nodes, comment)
			rawStart = p.pos
			continue
		}

		// Try parsing HTML elements
		element, err := p.parseElement()
		if err != nil {
			return nodes, err
		}
		if element != nil {
			nodes = append(nodes, element)
			rawStart = p.pos
			continue
		}

		// If nothing matched, advance one character
		if p.pos < p.length {
			p.pos++
		} else {
			break
		}
	}

	// Add any remaining raw text
	if p.pos > rawStart {
		text := p.input[rawStart:p.pos]
		if text != "" {
			nodes = append(nodes, &RawNode{
				Text: text,
				Line: p.getLineAt(rawStart),
			})
		}
	}

	return nodes, nil
}

// parseTemplateExpression parses a {{...}} template expression and returns a TemplateExpressionNode
func (p *Parser) parseTemplateExpression() (*TemplateExpressionNode, error) {
	if p.peek(2) != "{{" {
		//nolint: nilnil
		return nil, nil
	}

	startPos := p.pos
	p.pos += 2 // skip "{{"

	// Find the closing "}}"
	start := p.pos
	idx := strings.Index(p.input[p.pos:], "}}")
	if idx == -1 {
		return nil, fmt.Errorf("unterminated template expression starting at pos %d", startPos)
	}

	expression := p.input[start : start+idx]
	p.pos += idx + 2 // skip past "}}"

	return &TemplateExpressionNode{
		Expression: expression,
		Line:       p.getLineAt(startPos),
	}, nil
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
