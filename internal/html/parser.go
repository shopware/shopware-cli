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

// TraverseNode walks the AST and invokes f on every ElementNode it finds,
// including ones nested inside Twig structural nodes (block, if, for, embed,
// macro, etc.) and ones embedded in an element's Attributes slice (a Twig
// {% if %} can hold attribute-toggling element fragments). Downstream linters
// and fixers rely on this — adding a new structural node type means also
// teaching TraverseNode to recurse into its children.
func TraverseNode(n NodeList, f func(*ElementNode)) {
	for _, node := range n {
		switch node := node.(type) {
		case *ElementNode:
			f(node)
			TraverseNode(node.Attributes, f)
			TraverseNode(node.Children, f)
		case *TwigBlockNode:
			TraverseNode(node.Children, f)
		case *TwigIfNode:
			for _, br := range node.Branches {
				TraverseNode(br.Body, f)
			}
			TraverseNode(node.ElseChildren, f)
		case *TwigGenericBlockNode:
			TraverseNode(node.Body, f)
			TraverseNode(node.Else, f)
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

	// Per-type node slabs. RawNode, ElementNode and TemplateExpressionNode are
	// by far the most numerous node types; allocating them from slabs sized once
	// (in parseDocument, from the token count) turns ~one mallocgc per node into
	// an amortized slab bump. GC cost scales with the number of live heap
	// objects, so cutting object count is what actually moves wall-clock (a
	// single-threaded parse spends ~half its time in GC otherwise).
	rawSlab  []RawNode
	rawN     int
	elemSlab []ElementNode
	elemN    int
	exprSlab []TemplateExpressionNode
	exprN    int
	attrSlab []Attribute
	attrN    int

	// scratch is a single reused stack onto which parseNodesUntil builds each
	// node list. Recursive calls share the backing array via a mark (the length
	// at entry); collect() copies the [mark:] window into an exact-size NodeList
	// and rewinds. This replaces one append-grown slice per list — profiling's
	// single largest remaining allocator — with a handful of grows for the whole
	// parse.
	scratch []Node
}

func (p *parser) newRawNode() *RawNode {
	if p.rawN >= len(p.rawSlab) {
		n := len(p.rawSlab) * 2
		if n < 16 {
			n = 16
		}
		p.rawSlab = make([]RawNode, n)
		p.rawN = 0
	}
	r := &p.rawSlab[p.rawN]
	p.rawN++
	return r
}

func (p *parser) newElementNode() *ElementNode {
	if p.elemN >= len(p.elemSlab) {
		n := len(p.elemSlab) * 2
		if n < 8 {
			n = 8
		}
		p.elemSlab = make([]ElementNode, n)
		p.elemN = 0
	}
	e := &p.elemSlab[p.elemN]
	p.elemN++
	return e
}

func (p *parser) newExprNode() *TemplateExpressionNode {
	if p.exprN >= len(p.exprSlab) {
		n := len(p.exprSlab) * 2
		if n < 8 {
			n = 8
		}
		p.exprSlab = make([]TemplateExpressionNode, n)
		p.exprN = 0
	}
	e := &p.exprSlab[p.exprN]
	p.exprN++
	return e
}

func (p *parser) newAttrNode() *Attribute {
	if p.attrN >= len(p.attrSlab) {
		n := len(p.attrSlab) * 2
		if n < 16 {
			n = 16
		}
		p.attrSlab = make([]Attribute, n)
		p.attrN = 0
	}
	a := &p.attrSlab[p.attrN]
	p.attrN++
	return a
}

// collect copies the scratch entries pushed since mark into an exact-size
// NodeList and rewinds the scratch stack to mark for sibling reuse.
func (p *parser) collect(mark int) NodeList {
	n := len(p.scratch) - mark
	if n == 0 {
		p.scratch = p.scratch[:mark]
		return nil
	}
	out := make(NodeList, n)
	copy(out, p.scratch[mark:])
	p.scratch = p.scratch[:mark]
	return out
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
	// Pre-size the node slabs and the scratch stack from the token count. The
	// divisors come from measured node-per-token ratios on real templates
	// (~1 RawNode and ~1 ElementNode per 15 tokens, ~1 expression per 56);
	// right-sizing matters because an oversized make() is zeroed memory the GC
	// then has to scan every cycle. The slabs grow on demand if an estimate
	// runs short, so under-estimating is cheap and over-estimating is the costly
	// mistake.
	if n := len(toks)/15 + 8; n > 0 {
		p.rawSlab = make([]RawNode, n)
		p.elemSlab = make([]ElementNode, n)
	}
	// Storing attributes as *Attribute out of a slab turns what was one
	// interface-boxing malloc per attribute — profiling's largest remaining
	// allocator — into an amortized slab bump. The divisor is the measured
	// token-per-attribute ratio on real templates (~1 attribute per 32
	// tokens); slightly under-sizing keeps the initial slab from being zeroed
	// memory the GC must scan, and newAttrNode grows it on demand if short.
	if n := len(toks)/32 + 8; n > 0 {
		p.attrSlab = make([]Attribute, n)
	}
	if n := len(toks)/48 + 4; n > 0 {
		p.exprSlab = make([]TemplateExpressionNode, n)
	}
	// scratch only needs to hold one node list's worth at a time (collect()
	// rewinds between siblings), so it stays small.
	p.scratch = make([]Node, 0, len(toks)/16+16)
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

// rawSpan accumulates buffered raw text. The text folded into a RawNode is
// almost always a run of consecutive source bytes, so rawSpan records just the
// [start,end) offsets into src and materializes the RawNode as a zero-copy
// src[start:end] slice — no strings.Builder, no byte copy. It falls back to a
// Builder only if a non-contiguous append ever occurs, so correctness never
// depends on that assumption.
type rawSpan struct {
	src   string
	start int
	end   int
	has   bool
	bb    *strings.Builder // non-nil only after a non-contiguous append
}

// add appends the source bytes [off, off+n).
func (r *rawSpan) add(off, n int) {
	if n == 0 {
		return
	}
	if r.bb != nil {
		r.bb.WriteString(r.src[off : off+n])
		return
	}
	if !r.has {
		r.start, r.end, r.has = off, off+n, true
		return
	}
	if off == r.end {
		r.end = off + n
		return
	}
	r.bb = &strings.Builder{}
	r.bb.WriteString(r.src[r.start:r.end])
	r.bb.WriteString(r.src[off : off+n])
}

func (r *rawSpan) len() int {
	if r.bb != nil {
		return r.bb.Len()
	}
	if r.has {
		return r.end - r.start
	}
	return 0
}

func (r *rawSpan) text() string {
	if r.bb != nil {
		return r.bb.String()
	}
	if r.has {
		return r.src[r.start:r.end]
	}
	return ""
}

func (r *rawSpan) reset() {
	r.has = false
	r.bb = nil
}

// flushRaw pushes any buffered raw text onto the scratch stack as a RawNode.
func (p *parser) flushRaw(ctx nodeContext, rawBuf *rawSpan, rawStartPos Pos) {
	text := rawBuf.text()
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
		rn := p.newRawNode()
		rn.Text = text
		rn.Line = rawStartPos.Line
		p.scratch = append(p.scratch, rn)
	}
	rawBuf.reset()
}

// parseNodesUntil is the workhorse loop. It walks the token stream collecting
// nodes until EOF or until the parent tag's EndTag / Followers fire. closeTag
// is the HTML element close tag the children parser should stop on (only
// meaningful for nodeContextElementChildren). parentTagSpec is nil at the
// document root.
//
//nolint:gocyclo // top-level dispatcher; complexity is from one arm per token kind.
func (p *parser) parseNodesUntil(ctx nodeContext, closeTag string, parentTagSpec *TagSpec) (NodeList, stopReason, error) {
	mark := len(p.scratch)
	rawBuf := rawSpan{src: p.source}
	rawStartPos := p.peek(0).Pos

	for {
		tk := p.peek(0)
		if tk.Type == tokEOF {
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			return p.collect(mark), stopEOF, nil
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
					p.flushRaw(ctx, &rawBuf, rawStartPos)
					return p.collect(mark), stopGenericEndTag, nil
				}
				if isFollower(name, parentTagSpec.Followers) {
					p.flushRaw(ctx, &rawBuf, rawStartPos)
					return p.collect(mark), stopIfTerminator, nil
				}
			}
			// Top-level stop on {% endblock %}.
			if ctx == nodeContextTopLevel && name == "endblock" && parentTagSpec == nil {
				p.flushRaw(ctx, &rawBuf, rawStartPos)
				return p.collect(mark), stopEndblock, nil
			}

			spec := lookupTag(name)
			if spec == nil {
				// Unrecognized Twig tag — fold its raw bytes into the surrounding
				// text so unknown tags round-trip through Dump as their source bytes.
				flushBefore := rawBuf.len() == 0
				if flushBefore {
					rawStartPos = tk.Pos
				}
				p.appendRawTokens(&rawBuf, tk)
				continue
			}

			// Recognized tag: flush pending raw text, then parse.
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			node, err := spec.Parse(p, tk)
			if err != nil {
				return p.collect(mark), stopEOF, err
			}
			p.scratch = append(p.scratch, node)
			rawStartPos = p.peek(0).Pos

		case tokTwigExprOpen:
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			expr, err := p.parseTemplateExpression()
			if err != nil {
				return p.collect(mark), stopEOF, err
			}
			p.scratch = append(p.scratch, expr)
			rawStartPos = p.peek(0).Pos

		case tokTwigCommentOpen:
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			openPos := tk.Pos
			trim := TwigTrim{Left: tk.TrimLeft}
			p.advance() // {#
			body := ""
			if p.peek(0).Type == tokTwigCommentText {
				body = p.advance().Lit
			}
			if p.peek(0).Type == tokTwigCommentClose {
				trim.Right = p.peek(0).TrimRight
				p.advance()
			}
			p.scratch = append(p.scratch, &TwigCommentNode{Body: body, Trim: trim, Line: openPos.Line})
			rawStartPos = p.peek(0).Pos

		case tokHTMLComment:
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			p.scratch = append(p.scratch, &CommentNode{
				Text: tk.Lit,
				Line: tk.Pos.Line,
			})
			p.advance()
			rawStartPos = p.peek(0).Pos

		case tokHTMLOpenStart:
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			element, err := p.parseElement(parentTagSpec)
			if err != nil {
				return p.collect(mark), stopEOF, err
			}
			if element != nil {
				p.scratch = append(p.scratch, element)
			}
			rawStartPos = p.peek(0).Pos

		case tokHTMLCloseStart:
			// Possibly the close tag for our enclosing element.
			if ctx == nodeContextElementChildren {
				// Peek the tag name.
				nameTok := p.peek(1)
				if nameTok.Type == tokHTMLTagName && nameTok.Lit == closeTag {
					p.flushRaw(ctx, &rawBuf, rawStartPos)
					// Consume "</name>"
					p.advance() // </
					p.advance() // name
					if p.peek(0).Type == tokHTMLTagEnd {
						p.advance() // >
					}
					return p.collect(mark), stopElementCloseTag, nil
				}
			}
			// Otherwise treat as raw text (rare; mostly malformed).
			if rawBuf.len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.add(tk.Pos.Offset, len(tk.Raw))
			p.advance()

		case tokHTMLDoctype:
			p.flushRaw(ctx, &rawBuf, rawStartPos)
			dn := p.newRawNode()
			dn.Text = tk.Raw
			dn.Line = tk.Pos.Line
			p.scratch = append(p.scratch, dn)
			p.advance()
			rawStartPos = p.peek(0).Pos

		case tokText:
			if rawBuf.len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.add(tk.Pos.Offset, len(tk.Lit))
			p.advance()

		default:
			// Any other token (closer tokens, attr tokens out of place, etc.)
			// is folded into raw text.
			if rawBuf.len() == 0 {
				rawStartPos = tk.Pos
			}
			rawBuf.add(tk.Pos.Offset, len(tk.Raw))
			p.advance()
		}
	}
}

// appendRawTokens consumes a full {% ... %} or {{ ... }} or {# ... #} sequence
// starting at the current cursor and appends its raw bytes to buf. Used when
// the parser encounters a Twig tag that has no registered handler.
func (p *parser) appendRawTokens(buf *rawSpan, openTok token) {
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
		buf.add(openTok.Pos.Offset, len(openTok.Raw))
		p.advance()
		return
	}
	for {
		tk := p.peek(0)
		if tk.Type == tokEOF {
			return
		}
		buf.add(tk.Pos.Offset, len(tk.Raw))
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
	// Expression preserves the body bytes verbatim WITHOUT trimming; the
	// lexer's Lit on tokTwigRawExpr is the raw body, and we use Raw to keep
	// any leading/trailing spaces inside the {{ }} delimiters intact. The
	// trim flags on the open/close delimiters round-trip via Trim so a
	// `{{- x -}}` formats back as `{{- x -}}` and not `{{ x }}`.
	en := p.newExprNode()
	en.Expression = bodyTok.Raw
	en.Trim = TwigTrim{Left: openTok.TrimLeft, Right: closeTok.TrimRight}
	en.Line = openTok.Pos.Line
	return en, nil
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
	node := p.newElementNode()
	node.Tag = nameTok.Lit
	node.Attributes = NodeList{}
	node.Children = NodeList{}
	node.Line = openTok.Pos.Line

	// Parse attributes (including embedded Twig statements).
	for {
		tk := p.peek(0)
		// Attribute-loop dispatch. Stray tokens fall to default which
		// advances one position to make progress.
		//exhaustive:ignore
		switch tk.Type {
		case tokHTMLAttrName:
			p.advance()
			attr := p.newAttrNode()
			attr.Key = tk.Lit
			attr.Value = ""
			if p.peek(0).Type == tokHTMLAttrEq {
				p.advance() // =
				if p.peek(0).Type == tokHTMLAttrValue {
					attr.Value = p.advance().Lit
				}
			}
			node.Attributes = append(node.Attributes, attr)

		case tokTwigStmtOpen:
			// Embedded Twig directive inside an open tag. `{% if %}` becomes a
			// structured TwigIfNode in the attribute list (the formatter knows
			// how to render it across multiple lines). Other tags fall back
			// to a RawNode that preserves the original bytes verbatim —
			// dropping them would silently strip dynamic attributes like
			// `{% if x %}data-y{% endif %}` (when `if` is somehow missing)
			// or future Twig statements we don't yet recognize.
			identTok := p.peek(1)
			if identTok.Type == tokTwigIdent && identTok.Lit == "if" {
				if spec := lookupTag("if"); spec != nil {
					ifNode, err := spec.Parse(p, tk)
					if err != nil {
						return nil, err
					}
					node.Attributes = append(node.Attributes, ifNode)
					continue
				}
			}
			buf := rawSpan{src: p.source}
			startPos := tk.Pos
			p.appendRawTokens(&buf, tk)
			arn := p.newRawNode()
			arn.Text = buf.text()
			arn.Line = startPos.Line
			node.Attributes = append(node.Attributes, arn)

		case tokTwigExprOpen:
			// `<div {{ attributes }}>` — preserve the dynamic expression in
			// the attribute list so formatter passes don't strip it.
			expr, err := p.parseTemplateExpression()
			if err != nil {
				return nil, err
			}
			node.Attributes = append(node.Attributes, expr)

		case tokTwigCommentOpen:
			// `<div {# author note #}>` — preserve as a TwigCommentNode in
			// the attribute list.
			openPos := tk.Pos
			trim := TwigTrim{Left: tk.TrimLeft}
			p.advance()
			body := ""
			if p.peek(0).Type == tokTwigCommentText {
				body = p.advance().Lit
			}
			if p.peek(0).Type == tokTwigCommentClose {
				trim.Right = p.peek(0).TrimRight
				p.advance()
			}
			node.Attributes = append(node.Attributes, &TwigCommentNode{Body: body, Trim: trim, Line: openPos.Line})

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
