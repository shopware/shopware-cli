package html

import "strings"

func init() {
	registerTag(TagSpec{
		Name:   "block",
		EndTag: "endblock",
		Parse:  parseBlockTag,
	})
}

// parseBlockTag parses `{% block name %}...{% endblock %}`.
// On entry the cursor is on the OPEN token.
func parseBlockTag(p *parser, openTok token) (Node, error) {
	if openTok.Type != tokTwigStmtOpen {
		return nil, errAt(p.source, p.filename, openTok.Pos, "expected open delimiter for block tag")
	}
	openTrim := TwigTrim{Left: openTok.TrimLeft}
	p.advance() // {%
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit(p.source) != "block" {
		return nil, errAt(p.source, p.filename, identTok.Pos, "expected 'block' identifier")
	}
	bodyTok := p.advance()
	if bodyTok.Type != tokTwigRawExpr {
		return nil, errAt(p.source, p.filename, bodyTok.Pos, "expected block name")
	}
	closeTok := p.advance()
	if closeTok.Type != tokTwigStmtClose {
		return nil, errAt(p.source, p.filename, closeTok.Pos, "expected '%%}'")
	}
	openTrim.Right = closeTok.TrimRight

	// Block name is the first whitespace-delimited token of the body. Scan for
	// it directly rather than via strings.Fields, which would allocate a slice
	// for the whole body just to read fields[0].
	name := strings.TrimSpace(bodyTok.Lit(p.source))
	if i := strings.IndexFunc(name, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}); i != -1 {
		name = name[:i]
	}

	spec := lookupTag("block")
	children, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
	if err != nil {
		return nil, err
	}
	if reason != stopGenericEndTag {
		return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% endblock %%}")
	}

	closeTrim, err := p.consumeEndTag("endblock")
	if err != nil {
		return nil, err
	}

	return &TwigBlockNode{
		Name:      name,
		Children:  children,
		OpenTrim:  openTrim,
		CloseTrim: closeTrim,
		Line:      openTok.Pos.Line,
	}, nil
}

// consumeEndTag consumes `{% name %}` (with optional whitespace) and
// returns the trim flags from its open/close delimiters.
func (p *parser) consumeEndTag(name string) (TwigTrim, error) {
	openTok := p.peek(0)
	if openTok.Type != tokTwigStmtOpen {
		return TwigTrim{}, errAt(p.source, p.filename, openTok.Pos, "expected '{%%' for end tag '%s'", name)
	}
	trim := TwigTrim{Left: openTok.TrimLeft}
	p.advance()
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit(p.source) != name {
		return TwigTrim{}, errAt(p.source, p.filename, identTok.Pos, "expected '%s'", name)
	}
	// Body is usually empty; skip.
	if p.peek(0).Type == tokTwigRawExpr {
		p.advance()
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return TwigTrim{}, errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' to close '%s'", name)
	}
	trim.Right = closeTok.TrimRight
	p.advance()
	return trim, nil
}
