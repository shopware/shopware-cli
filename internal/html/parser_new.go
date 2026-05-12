package html

// parser is the new token-driven parser, currently scaffolding only.
//
// Status: the lexer (lexer.go), token model (tokens.go), Pos (pos.go) and
// ParseError (errors.go) are in place and exercised by lexer_test.go. The
// next step is to wire this struct into NewParser / NewAdminParser /
// NewStorefrontParser, replacing the byte-indexed parser in parser.go.
//
// Until that lands, the public entry points still call the legacy
// (parser.go) implementation, so all 45 fixtures continue to format
// byte-identically.
type parser struct {
	source   string
	filename string
	tokens   []token
	idx      int
}

// peek returns the token at offset i ahead of the cursor, or an EOF token
// past the end.
func (p *parser) peek(i int) token {
	pos := p.idx + i
	if pos >= len(p.tokens) {
		// Synthetic EOF at end of source.
		var lastPos Pos
		if n := len(p.tokens); n > 0 {
			lastPos = p.tokens[n-1].Pos
		}
		return token{Type: tokEOF, Pos: lastPos}
	}
	return p.tokens[pos]
}

// advance consumes one token and returns it.
func (p *parser) advance() token {
	t := p.peek(0)
	if t.Type != tokEOF {
		p.idx++
	}
	return t
}

// errf constructs a ParseError at the current cursor position.
func (p *parser) errf(format string, args ...any) *ParseError {
	return errAt(p.source, p.filename, p.peek(0).Pos, format, args...)
}
