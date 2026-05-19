package html

import "strings"

func init() {
	registerTag(TagSpec{
		Name:  "parent",
		Parse: parseParentTag,
	})
}

// parseParentTag handles both `{% parent %}` and `{% parent() %}`.
// Anything else after `parent` (e.g. `{% parent foo %}`) is a parse error —
// silently rewriting it to `{% parent() %}` would hide real authoring
// mistakes from the verifier and formatter pipelines.
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
	// Body must be empty or exactly "()" (optionally surrounded by
	// whitespace). Anything else is rejected to avoid silently rewriting
	// malformed tags.
	if p.peek(0).Type == tokTwigRawExpr {
		bodyTok := p.advance()
		if body := strings.TrimSpace(bodyTok.Lit); body != "" && body != "()" {
			return nil, errAt(p.source, p.filename, bodyTok.Pos, "unexpected argument to parent: %q", body)
		}
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return nil, errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' for parent tag")
	}
	trim.Right = closeTok.TrimRight
	p.advance()
	return &ParentNode{Trim: trim, Line: startLine}, nil
}
