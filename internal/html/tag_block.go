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
	p.advance() // {%
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit != "block" {
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

	// Block name is the first whitespace-delimited token of the body.
	name := strings.TrimSpace(bodyTok.Lit)
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}

	spec := lookupTag("block")
	children, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
	if err != nil {
		return nil, err
	}
	if reason != stopGenericEndTag {
		return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% endblock %%}")
	}

	// Consume `{% endblock %}`.
	if err := p.consumeEndTag("endblock"); err != nil {
		return nil, err
	}

	return &TwigBlockNode{
		Name:     name,
		Children: children,
		Line:     openTok.Pos.Line,
	}, nil
}

// consumeEndTag consumes `{% name %}` (with optional whitespace).
func (p *parser) consumeEndTag(name string) error {
	openTok := p.peek(0)
	if openTok.Type != tokTwigStmtOpen {
		return errAt(p.source, p.filename, openTok.Pos, "expected '{%%' for end tag '%s'", name)
	}
	p.advance()
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit != name {
		return errAt(p.source, p.filename, identTok.Pos, "expected '%s'", name)
	}
	// Body is usually empty; skip.
	if p.peek(0).Type == tokTwigRawExpr {
		p.advance()
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' to close '%s'", name)
	}
	p.advance()
	return nil
}
