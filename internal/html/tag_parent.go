package html

func init() {
	registerTag(TagSpec{
		Name:  "parent",
		Parse: parseParentTag,
	})
}

// parseParentTag handles both `{% parent %}` and `{% parent() %}`.
func parseParentTag(p *parser, openTok token) (Node, error) {
	if openTok.Type != tokTwigStmtOpen {
		return nil, errAt(p.source, p.filename, openTok.Pos, "expected open delimiter for parent tag")
	}
	startLine := openTok.Pos.Line
	trim := TwigTrim{Left: openTok.TrimLeft}
	p.advance() // {%
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit != "parent" {
		return nil, errAt(p.source, p.filename, identTok.Pos, "expected 'parent'")
	}
	// Body may be empty or "()".
	if p.peek(0).Type == tokTwigRawExpr {
		p.advance()
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return nil, errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' for parent tag")
	}
	trim.Right = closeTok.TrimRight
	p.advance()
	return &ParentNode{Trim: trim, Line: startLine}, nil
}
