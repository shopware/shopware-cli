package html

import "strings"

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

// parser is the token-driven parser. The exported entry points
// (NewParser, NewAdminParser, NewStorefrontParser) construct one and call
// parseDocument.
type parser struct {
	source   string
	filename string
	tokens   []token
	idx      int
}

func (p *parser) peek(i int) token {
	pos := p.idx + i
	if pos >= len(p.tokens) {
		var lastPos Pos
		if n := len(p.tokens); n > 0 {
			lastPos = p.tokens[n-1].Pos
		}
		return token{Type: tokEOF, Pos: lastPos}
	}
	return p.tokens[pos]
}

func (p *parser) advance() token {
	t := p.peek(0)
	if t.Type != tokEOF {
		p.idx++
	}
	return t
}

// parseDocument runs the lexer on the source and parses a full document
// (top-level nodes). It is the body of the public NewParser entry point.
func (p *parser) parseDocument() (NodeList, error) {
	lex := newLexer(p.source)
	toks, err := lex.lex()
	if err != nil {
		return nil, err
	}
	p.tokens = toks
	p.idx = 0
	nodes, _, err := p.parseNodesUntil(nodeContextTopLevel, "", nil)
	return nodes, err
}

// nodeContext controls RawNode chunking behavior. At the top level
// (and inside Twig blocks / if-branches) we trim whitespace before
// deciding whether to emit a RawNode between two delimiters. Inside an
// HTML element's children we keep whitespace-only RawNodes so the
// formatter can faithfully reproduce inter-tag spacing.
type nodeContext int

const (
	nodeContextTopLevel        nodeContext = iota // top level / inside {% block %} / inside if branch
	nodeContextElementChildren                    // inside an HTML element's children
)

// stopReason indicates why parseNodesUntil returned.
type stopReason int

const (
	stopEOF             stopReason = iota
	stopEndblock                   // saw {% endblock %} (consumed)
	stopIfTerminator               // saw {% else / elseif / endif %} (NOT consumed)
	stopGenericEndTag              // saw a registered EndTag for the parent (NOT consumed)
	stopElementCloseTag            // saw </name> matching closeTag (consumed)
)

// parseNodesUntil is the workhorse loop. It walks the token stream collecting
// nodes until EOF or until the parent tag's EndTag / Followers fire. closeTag
// is the HTML element close tag the children parser should stop on (only
// meaningful for nodeContextElementChildren). parentTagSpec is nil at the
// document root.
//
//nolint:gocyclo // top-level dispatcher; complexity is from one arm per token kind.
func (p *parser) parseNodesUntil(ctx nodeContext, closeTag string, parentTagSpec *TagSpec) (NodeList, stopReason, error) {
	var nodes NodeList
	var rawBuf strings.Builder
	rawStartPos := p.peek(0).Pos

	flush := func() {
		text := rawBuf.String()
		if text == "" {
			return
		}
		shouldEmit := false
		switch ctx {
		case nodeContextTopLevel:
			// Trim-space check at top-level: keep RawNodes only when they
			// carry non-whitespace text so blank gaps between block tags
			// don't materialize as empty nodes.
			shouldEmit = strings.TrimSpace(text) != ""
		case nodeContextElementChildren:
			shouldEmit = text != ""
		}
		if shouldEmit {
			nodes = append(nodes, &RawNode{Text: text, Line: rawStartPos.Line})
		}
		rawBuf.Reset()
	}

	for {
		tk := p.peek(0)
		if tk.Type == tokEOF {
			flush()
			return nodes, stopEOF, nil
		}

		// Token dispatch. Cases not listed fall to default and get folded
		// into the surrounding RawNode (whitespace, stray attr tokens, etc.).
		//exhaustive:ignore
		switch tk.Type {
		case tokTwigStmtOpen:
			// Look at the identifier (next non-whitespace token).
			identTok := p.peek(1)
			name := ""
			if identTok.Type == tokTwigIdent {
				name = identTok.Lit
			}

			// Stop on parent's terminator/followers without consuming.
			if parentTagSpec != nil {
				if name == parentTagSpec.EndTag {
					flush()
					return nodes, stopGenericEndTag, nil
				}
				if isFollower(name, parentTagSpec.Followers) {
					flush()
					return nodes, stopIfTerminator, nil
				}
			}
			// Top-level stop on {% endblock %}.
			if ctx == nodeContextTopLevel && name == "endblock" && parentTagSpec == nil {
				flush()
				return nodes, stopEndblock, nil
			}

			spec := lookupTag(name)
			if spec == nil {
				// Unrecognized Twig tag — fold its raw bytes into the surrounding
				// text so unknown tags round-trip through Dump as their source bytes.
				flushBefore := rawBuf.Len() == 0
				if flushBefore {
					rawStartPos = tk.Pos
				}
				p.appendRawTokens(&rawBuf, tk)
				continue
			}

			// Recognized tag: flush pending raw text, then parse.
			flush()
			node, err := spec.Parse(p, tk)
			if err != nil {
				return nodes, stopEOF, err
			}
			nodes = append(nodes, node)
			rawStartPos = p.peek(0).Pos

		case tokTwigExprOpen:
			flush()
			expr, err := p.parseTemplateExpression()
			if err != nil {
				return nodes, stopEOF, err
			}
			nodes = append(nodes, expr)
			rawStartPos = p.peek(0).Pos

		case tokTwigCommentOpen:
			flush()
			openPos := tk.Pos
			p.advance() // {#
			body := ""
			if p.peek(0).Type == tokTwigCommentText {
				body = p.advance().Lit
			}
			if p.peek(0).Type == tokTwigCommentClose {
				p.advance()
			}
			nodes = append(nodes, &TwigCommentNode{Body: body, Line: openPos.Line})
			rawStartPos = p.peek(0).Pos

		case tokHTMLComment:
			flush()
			nodes = append(nodes, &CommentNode{
				Text: tk.Lit,
				Line: tk.Pos.Line,
			})
			p.advance()
			rawStartPos = p.peek(0).Pos

		case tokHTMLOpenStart:
			flush()
			element, err := p.parseElement(parentTagSpec)
			if err != nil {
				return nodes, stopEOF, err
			}
			if element != nil {
				nodes = append(nodes, element)
			}
			rawStartPos = p.peek(0).Pos

		case tokHTMLCloseStart:
			// Possibly the close tag for our enclosing element.
			if ctx == nodeContextElementChildren {
				// Peek the tag name.
				nameTok := p.peek(1)
				if nameTok.Type == tokHTMLTagName && nameTok.Lit == closeTag {
					flush()
					// Consume "</name>"
					p.advance() // </
					p.advance() // name
					if p.peek(0).Type == tokHTMLTagEnd {
						p.advance() // >
					}
					return nodes, stopElementCloseTag, nil
				}
			}
			// Otherwise treat as raw text (rare; mostly malformed).
			if rawBuf.Len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.WriteString(tk.Raw)
			p.advance()

		case tokHTMLDoctype:
			flush()
			nodes = append(nodes, &RawNode{Text: tk.Raw, Line: tk.Pos.Line})
			p.advance()
			rawStartPos = p.peek(0).Pos

		case tokText:
			if rawBuf.Len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.WriteString(tk.Lit)
			p.advance()

		default:
			// Any other token (closer tokens, attr tokens out of place, etc.)
			// is folded into raw text.
			if rawBuf.Len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.WriteString(tk.Raw)
			p.advance()
		}
	}
}

// appendRawTokens consumes a full {% ... %} or {{ ... }} or {# ... #} sequence
// starting at the current cursor and appends its raw bytes to buf. Used when
// the parser encounters a Twig tag that has no registered handler.
func (p *parser) appendRawTokens(buf *strings.Builder, openTok token) {
	var wantClose tokenType
	// Only the three Twig opener token types are valid here; anything else
	// is a caller bug and falls through to the default arm.
	//exhaustive:ignore
	switch openTok.Type {
	case tokTwigStmtOpen:
		wantClose = tokTwigStmtClose
	case tokTwigExprOpen:
		wantClose = tokTwigExprClose
	case tokTwigCommentOpen:
		wantClose = tokTwigCommentClose
	default:
		buf.WriteString(openTok.Raw)
		p.advance()
		return
	}
	for {
		tk := p.peek(0)
		if tk.Type == tokEOF {
			return
		}
		buf.WriteString(tk.Raw)
		p.advance()
		if tk.Type == wantClose {
			return
		}
	}
}

// parseTemplateExpression consumes a `{{ ... }}` triplet and returns a node.
func (p *parser) parseTemplateExpression() (*TemplateExpressionNode, error) {
	openTok := p.advance() // {{
	if openTok.Type != tokTwigExprOpen {
		return nil, errAt(p.source, p.filename, openTok.Pos, "expected '{{'")
	}
	bodyTok := p.advance()
	if bodyTok.Type != tokTwigRawExpr {
		return nil, errAt(p.source, p.filename, bodyTok.Pos, "expected expression body")
	}
	closeTok := p.advance()
	if closeTok.Type != tokTwigExprClose {
		return nil, errAt(p.source, p.filename, closeTok.Pos, "expected '}}'")
	}
	// Legacy parser preserved spacing inside {{ }}: Expression is the body
	// verbatim WITHOUT trimming. The lexer's Lit on tokTwigRawExpr is the raw
	// body; for {{ }} we use Raw to preserve leading/trailing spaces exactly.
	return &TemplateExpressionNode{
		Expression: bodyTok.Raw,
		Line:       openTok.Pos.Line,
	}, nil
}

// parseElement consumes `<tag attrs...>children</tag>` or `<tag .../>`.
// parentTagSpec is the Twig tag spec of the enclosing scope (if any). It is
// threaded down so that element-children parsing can yield on an outer
// Twig terminator like `{% endblock %}` when a template wraps just the
// opening or closing HTML tag in a control-flow block. See the comment on
// parseElement's children call below.
func (p *parser) parseElement(parentTagSpec *TagSpec) (*ElementNode, error) {
	openTok := p.advance() // <
	if openTok.Type != tokHTMLOpenStart {
		return nil, errAt(p.source, p.filename, openTok.Pos, "expected '<'")
	}
	nameTok := p.advance()
	if nameTok.Type != tokHTMLTagName {
		return nil, errAt(p.source, p.filename, nameTok.Pos, "expected HTML tag name")
	}
	node := &ElementNode{
		Tag:        nameTok.Lit,
		Attributes: NodeList{},
		Children:   NodeList{},
		Line:       openTok.Pos.Line,
	}

	// Parse attributes (including embedded Twig statements).
	for {
		tk := p.peek(0)
		// Attribute-loop dispatch. Stray tokens fall to default which
		// advances one position to make progress.
		//exhaustive:ignore
		switch tk.Type {
		case tokHTMLAttrName:
			p.advance()
			attr := Attribute{Key: tk.Lit}
			if p.peek(0).Type == tokHTMLAttrEq {
				p.advance() // =
				if p.peek(0).Type == tokHTMLAttrValue {
					attr.Value = p.advance().Lit
				}
			}
			node.Attributes = append(node.Attributes, attr)

		case tokTwigStmtOpen:
			// Embedded Twig directive inside an open tag. Only `{% if %}`
			// produces a structured Attribute child (a TwigIfNode in the
			// attribute list); other tags are dropped to keep the
			// attribute list semantically clean.
			identTok := p.peek(1)
			if identTok.Type == tokTwigIdent && identTok.Lit == "if" {
				spec := lookupTag("if")
				if spec != nil {
					ifNode, err := spec.Parse(p, tk)
					if err != nil {
						return nil, err
					}
					node.Attributes = append(node.Attributes, ifNode)
					continue
				}
			}
			// Unrecognized in attribute context — skip to its close to avoid
			// infinite loops.
			var buf strings.Builder
			p.appendRawTokens(&buf, tk)

		case tokHTMLTagEnd:
			p.advance() // >
			if isVoidElement(node.Tag) {
				node.SelfClosing = true
				return node, nil
			}
			// Pass parentTagSpec down so the children parser yields on an
			// outer Twig terminator. Real-world templates often wrap just
			// the opening (or just the closing) HTML tag in {% if %} or
			// {% block %}, leaving the element's children syntactically
			// outside the Twig control-flow scope. Without this, the
			// element children parser would consume {% endif %} / {%
			// endblock %} as raw text while searching for </tag>, then
			// the outer Twig tag could not find its terminator.
			children, reason, err := p.parseNodesUntil(nodeContextElementChildren, node.Tag, parentTagSpec)
			if err != nil {
				return nil, err
			}
			// If we stopped on something other than </tag>, the element is
			// unclosed in this control-flow scope; its </tag> appears later
			// as raw text and the formatter should not synthesize one.
			// Also drop any whitespace-only RawNode tail that the children
			// loop accumulated before the outer Twig terminator — those
			// bytes belong to the outer scope's indentation, not the
			// element's body, and would otherwise round-trip as a stray
			// indented blank line.
			if reason != stopElementCloseTag {
				node.Unclosed = true
				for len(children) > 0 {
					if raw, ok := children[len(children)-1].(*RawNode); ok && strings.TrimSpace(raw.Text) == "" {
						children = children[:len(children)-1]
						continue
					}
					break
				}
			}
			node.Children = children
			return node, nil

		case tokHTMLSelfClose:
			p.advance() // />
			node.SelfClosing = true
			return node, nil

		case tokEOF:
			return nil, errAt(p.source, p.filename, tk.Pos, "unterminated HTML open tag")

		default:
			// Advance to avoid infinite loop on unexpected token.
			p.advance()
		}
	}
}
