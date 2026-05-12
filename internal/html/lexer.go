package html

import (
	"strings"
)

// lexer scans the source into a stream of tokens. It interleaves HTML and Twig
// recognition. Twig statement/expression/comment bodies are returned as a single
// opaque token (tokTwigRawExpr / tokTwigCommentText) — the parser does not
// understand Twig expression syntax beyond what is needed for tag dispatch.
type lexer struct {
	src    string
	pt     *posTracker
	tokens []token
}

func newLexer(src string) *lexer {
	return &lexer{src: src, pt: newPosTracker(src)}
}

// lex scans the entire source and returns the token stream.
func (l *lexer) lex() ([]token, error) {
	for l.pt.cur.Offset < len(l.src) {
		if err := l.lexContent(); err != nil {
			return nil, err
		}
	}
	l.emit(token{Type: tokEOF, Pos: l.pt.pos()})
	return l.tokens, nil
}

func (l *lexer) emit(t token) {
	l.tokens = append(l.tokens, t)
}

// remaining returns the unconsumed source.
func (l *lexer) remaining() string {
	return l.src[l.pt.cur.Offset:]
}

// peekStr returns the next n bytes without advancing.
func (l *lexer) peekStr(n int) string {
	r := l.remaining()
	if len(r) < n {
		return r
	}
	return r[:n]
}

// peekByte returns the byte at offset i ahead of the cursor, or 0 if past end.
func (l *lexer) peekByte(i int) byte {
	off := l.pt.cur.Offset + i
	if off >= len(l.src) {
		return 0
	}
	return l.src[off]
}

// lexContent scans raw text/HTML markup/Twig delimiters at the top level.
// It detects the next interesting boundary and dispatches to the right scanner.
func (l *lexer) lexContent() error {
	startPos := l.pt.pos()
	rem := l.remaining()
	if rem == "" {
		return nil
	}

	// Find the next delimiter we care about.
	idx := -1
	kind := ""
	for i := 0; i < len(rem); i++ {
		c := rem[i]
		switch c {
		case '<':
			if strings.HasPrefix(rem[i:], "<!--") {
				idx, kind = i, "html-comment"
			} else if strings.HasPrefix(rem[i:], "<!DOCTYPE") || strings.HasPrefix(rem[i:], "<!doctype") {
				idx, kind = i, "html-doctype"
			} else if i+1 < len(rem) && rem[i+1] == '/' {
				if i+2 < len(rem) && isHTMLNameStart(rem[i+2]) {
					idx, kind = i, "html-close"
				}
			} else if i+1 < len(rem) && isHTMLNameStart(rem[i+1]) {
				idx, kind = i, "html-open"
			}
		case '{':
			if i+1 < len(rem) {
				switch rem[i+1] {
				case '%':
					idx, kind = i, "twig-stmt"
				case '{':
					idx, kind = i, "twig-expr"
				case '#':
					idx, kind = i, "twig-comment"
				}
			}
		}
		if idx != -1 {
			break
		}
	}

	if idx == -1 {
		// Rest is all text.
		l.emit(token{Type: tokText, Lit: rem, Raw: rem, Pos: startPos})
		l.pt.advance(len(rem))
		return nil
	}

	if idx > 0 {
		text := rem[:idx]
		l.emit(token{Type: tokText, Lit: text, Raw: text, Pos: startPos})
		l.pt.advance(idx)
	}

	switch kind {
	case "html-comment":
		return l.lexHTMLComment()
	case "html-doctype":
		return l.lexHTMLDoctype()
	case "html-open":
		return l.lexHTMLOpenTag()
	case "html-close":
		return l.lexHTMLCloseTag()
	case "twig-stmt":
		return l.lexTwigStmt()
	case "twig-expr":
		return l.lexTwigExpr()
	case "twig-comment":
		return l.lexTwigComment()
	}
	return nil
}

func isHTMLNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isHTMLNameRune(c byte) bool {
	return isHTMLNameStart(c) || (c >= '0' && c <= '9') || c == '-' || c == ':' || c == '.'
}

func isTwigIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isTwigIdentRune(c byte) bool {
	return isTwigIdentStart(c) || (c >= '0' && c <= '9')
}

func isASCIIWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// lexHTMLComment scans <!-- ... -->. Body is preserved verbatim.
func (l *lexer) lexHTMLComment() error {
	startPos := l.pt.pos()
	rem := l.remaining()
	end := strings.Index(rem, "-->")
	if end == -1 {
		return &ParseError{Pos: startPos, Msg: "unterminated HTML comment", Source: l.src}
	}
	raw := rem[:end+3]
	body := strings.TrimSpace(rem[4:end])
	l.emit(token{Type: tokHTMLComment, Lit: body, Raw: raw, Pos: startPos})
	l.pt.advance(len(raw))
	return nil
}

// lexHTMLDoctype scans <!DOCTYPE ...>.
func (l *lexer) lexHTMLDoctype() error {
	startPos := l.pt.pos()
	rem := l.remaining()
	end := strings.Index(rem, ">")
	if end == -1 {
		return &ParseError{Pos: startPos, Msg: "unterminated <!DOCTYPE>", Source: l.src}
	}
	raw := rem[:end+1]
	l.emit(token{Type: tokHTMLDoctype, Lit: raw, Raw: raw, Pos: startPos})
	l.pt.advance(len(raw))
	return nil
}

// lexHTMLOpenTag emits tokens for `<name attr="val" {% if x %}...{% endif %} ...>` or `... />`.
func (l *lexer) lexHTMLOpenTag() error {
	startPos := l.pt.pos()
	l.emit(token{Type: tokHTMLOpenStart, Lit: "<", Raw: "<", Pos: startPos})
	l.pt.advance(1) // skip '<'

	if err := l.lexHTMLTagName(); err != nil {
		return err
	}
	return l.lexHTMLAttrsAndClose()
}

// lexHTMLCloseTag emits tokens for `</name>`.
func (l *lexer) lexHTMLCloseTag() error {
	startPos := l.pt.pos()
	l.emit(token{Type: tokHTMLCloseStart, Lit: "</", Raw: "</", Pos: startPos})
	l.pt.advance(2)
	l.skipASCIIWhitespace()
	if err := l.lexHTMLTagName(); err != nil {
		return err
	}
	l.skipASCIIWhitespace()
	if l.peekByte(0) != '>' {
		return &ParseError{Pos: l.pt.pos(), Msg: "expected '>' for closing tag", Source: l.src}
	}
	endPos := l.pt.pos()
	l.emit(token{Type: tokHTMLTagEnd, Lit: ">", Raw: ">", Pos: endPos})
	l.pt.advance(1)
	return nil
}

func (l *lexer) lexHTMLTagName() error {
	startPos := l.pt.pos()
	start := l.pt.cur.Offset
	if start >= len(l.src) || !isHTMLNameStart(l.src[start]) {
		return &ParseError{Pos: startPos, Msg: "expected HTML tag name", Source: l.src}
	}
	end := start
	for end < len(l.src) && isHTMLNameRune(l.src[end]) {
		end++
	}
	name := l.src[start:end]
	l.emit(token{Type: tokHTMLTagName, Lit: name, Raw: name, Pos: startPos})
	l.pt.advance(end - start)
	return nil
}

func (l *lexer) skipASCIIWhitespace() {
	for l.pt.cur.Offset < len(l.src) && isASCIIWhitespace(l.src[l.pt.cur.Offset]) {
		l.pt.advance(1)
	}
}

// lexHTMLAttrsAndClose scans attributes (including embedded Twig statements
// allowed mid-tag) until it reaches '>' or '/>'.
func (l *lexer) lexHTMLAttrsAndClose() error {
	for {
		l.skipASCIIWhitespace()
		c := l.peekByte(0)
		if c == 0 {
			return &ParseError{Pos: l.pt.pos(), Msg: "unterminated HTML open tag", Source: l.src}
		}
		if c == '>' {
			pos := l.pt.pos()
			l.emit(token{Type: tokHTMLTagEnd, Lit: ">", Raw: ">", Pos: pos})
			l.pt.advance(1)
			return nil
		}
		if c == '/' && l.peekByte(1) == '>' {
			pos := l.pt.pos()
			l.emit(token{Type: tokHTMLSelfClose, Lit: "/>", Raw: "/>", Pos: pos})
			l.pt.advance(2)
			return nil
		}
		// Twig statement inside an open tag (e.g. attribute toggles).
		if c == '{' && l.peekByte(1) == '%' {
			if err := l.lexTwigStmt(); err != nil {
				return err
			}
			continue
		}
		if c == '{' && l.peekByte(1) == '{' {
			if err := l.lexTwigExpr(); err != nil {
				return err
			}
			continue
		}
		if err := l.lexHTMLAttr(); err != nil {
			return err
		}
	}
}

func (l *lexer) lexHTMLAttr() error {
	startPos := l.pt.pos()
	start := l.pt.cur.Offset
	for l.pt.cur.Offset < len(l.src) {
		c := l.src[l.pt.cur.Offset]
		if isASCIIWhitespace(c) || c == '=' || c == '>' || c == '/' {
			break
		}
		l.pt.advance(1)
	}
	name := l.src[start:l.pt.cur.Offset]
	if name == "" {
		// Unexpected character. Skip one byte to make progress and let the
		// parser raise an error later.
		l.pt.advance(1)
		return nil
	}
	l.emit(token{Type: tokHTMLAttrName, Lit: name, Raw: name, Pos: startPos})
	l.skipASCIIWhitespace()
	if l.peekByte(0) != '=' {
		return nil
	}
	eqPos := l.pt.pos()
	l.emit(token{Type: tokHTMLAttrEq, Lit: "=", Raw: "=", Pos: eqPos})
	l.pt.advance(1)
	l.skipASCIIWhitespace()
	return l.lexHTMLAttrValue()
}

func (l *lexer) lexHTMLAttrValue() error {
	startPos := l.pt.pos()
	c := l.peekByte(0)
	if c == '"' || c == '\'' {
		quote := c
		l.pt.advance(1)
		start := l.pt.cur.Offset
		for l.pt.cur.Offset < len(l.src) && l.src[l.pt.cur.Offset] != quote {
			l.pt.advance(1)
		}
		val := l.src[start:l.pt.cur.Offset]
		if l.pt.cur.Offset < len(l.src) && l.src[l.pt.cur.Offset] == quote {
			l.pt.advance(1)
		}
		l.emit(token{Type: tokHTMLAttrValue, Lit: val, Raw: val, Pos: startPos, QuoteChar: quote})
		return nil
	}
	// Bareword.
	start := l.pt.cur.Offset
	for l.pt.cur.Offset < len(l.src) {
		b := l.src[l.pt.cur.Offset]
		if isASCIIWhitespace(b) || b == '>' || b == '/' {
			break
		}
		l.pt.advance(1)
	}
	val := l.src[start:l.pt.cur.Offset]
	l.emit(token{Type: tokHTMLAttrValue, Lit: val, Raw: val, Pos: startPos, QuoteChar: 0})
	return nil
}

// lexTwigStmt emits the open/ident/raw-body/close tokens for `{% ... %}`.
func (l *lexer) lexTwigStmt() error {
	openPos := l.pt.pos()
	trimLeft := false
	openLen := 2
	if l.peekByte(2) == '-' {
		trimLeft = true
		openLen = 3
	}
	openRaw := l.src[l.pt.cur.Offset : l.pt.cur.Offset+openLen]
	l.emit(token{Type: tokTwigStmtOpen, Lit: openRaw, Raw: openRaw, Pos: openPos, TrimLeft: trimLeft})
	l.pt.advance(openLen)

	// Identifier (tag name) — capture any leading whitespace in Raw.
	wsStart := l.pt.cur.Offset
	wsPos := l.pt.pos()
	for l.pt.cur.Offset < len(l.src) && isASCIIWhitespace(l.src[l.pt.cur.Offset]) {
		l.pt.advance(1)
	}
	identStart := l.pt.cur.Offset
	for l.pt.cur.Offset < len(l.src) && isTwigIdentRune(l.src[l.pt.cur.Offset]) {
		l.pt.advance(1)
	}
	ident := l.src[identStart:l.pt.cur.Offset]
	identRaw := l.src[wsStart:l.pt.cur.Offset]
	if ident != "" || identRaw != "" {
		l.emit(token{Type: tokTwigIdent, Lit: ident, Raw: identRaw, Pos: wsPos})
	}

	bodyStart := l.pt.cur.Offset
	bodyPos := l.pt.pos()
	closeOffset, trimRight, err := scanToTwigClose(l.src, l.pt.cur.Offset, "%}")
	if err != nil {
		return err
	}
	if closeOffset == -1 {
		return &ParseError{Pos: openPos, Msg: "unterminated {% ... %}", Source: l.src}
	}
	rawBody := l.src[bodyStart:closeOffset]
	body := strings.TrimSpace(rawBody)
	l.emit(token{Type: tokTwigRawExpr, Lit: body, Raw: rawBody, Pos: bodyPos})
	l.pt.advance(closeOffset - l.pt.cur.Offset)

	closeLen := 2
	if trimRight {
		closeLen = 3
	}
	closePos := l.pt.pos()
	closeRaw := l.src[l.pt.cur.Offset : l.pt.cur.Offset+closeLen]
	l.emit(token{Type: tokTwigStmtClose, Lit: closeRaw, Raw: closeRaw, Pos: closePos, TrimRight: trimRight})
	l.pt.advance(closeLen)
	return nil
}

// lexTwigExpr emits tokens for `{{ ... }}`.
func (l *lexer) lexTwigExpr() error {
	openPos := l.pt.pos()
	trimLeft := false
	openLen := 2
	if l.peekByte(2) == '-' {
		trimLeft = true
		openLen = 3
	}
	l.emit(token{Type: tokTwigExprOpen, Lit: l.src[l.pt.cur.Offset : l.pt.cur.Offset+openLen], Raw: l.src[l.pt.cur.Offset : l.pt.cur.Offset+openLen], Pos: openPos, TrimLeft: trimLeft})
	l.pt.advance(openLen)

	bodyStart := l.pt.cur.Offset
	bodyPos := l.pt.pos()
	closeOffset, trimRight, err := scanToTwigClose(l.src, l.pt.cur.Offset, "}}")
	if err != nil {
		return err
	}
	if closeOffset == -1 {
		return &ParseError{Pos: openPos, Msg: "unterminated {{ ... }}", Source: l.src}
	}
	rawBody := l.src[bodyStart:closeOffset]
	body := rawBody
	if trimRight {
		// trimRight strip: scanToTwigClose returns offset pointing at the '-' of '-}}'.
		// Body should keep its leading/trailing spaces around the inner expression
		// to mirror the original (the parser later strips when emitting).
		body = strings.TrimSuffix(strings.TrimRight(body, " \t"), "-")
	}
	l.emit(token{Type: tokTwigRawExpr, Lit: body, Raw: rawBody, Pos: bodyPos})
	l.pt.advance(closeOffset - l.pt.cur.Offset)

	closeLen := 2
	if trimRight {
		closeLen = 3
	}
	closePos := l.pt.pos()
	l.emit(token{Type: tokTwigExprClose, Lit: l.src[l.pt.cur.Offset : l.pt.cur.Offset+closeLen], Raw: l.src[l.pt.cur.Offset : l.pt.cur.Offset+closeLen], Pos: closePos, TrimRight: trimRight})
	l.pt.advance(closeLen)
	return nil
}

// lexTwigComment emits tokens for `{# ... #}`.
func (l *lexer) lexTwigComment() error {
	openPos := l.pt.pos()
	trimLeft := false
	openLen := 2
	if l.peekByte(2) == '-' {
		trimLeft = true
		openLen = 3
	}
	l.emit(token{Type: tokTwigCommentOpen, Lit: l.src[l.pt.cur.Offset : l.pt.cur.Offset+openLen], Raw: l.src[l.pt.cur.Offset : l.pt.cur.Offset+openLen], Pos: openPos, TrimLeft: trimLeft})
	l.pt.advance(openLen)

	bodyStart := l.pt.cur.Offset
	bodyPos := l.pt.pos()
	rem := l.src[l.pt.cur.Offset:]
	end := strings.Index(rem, "#}")
	if end == -1 {
		return &ParseError{Pos: openPos, Msg: "unterminated {# ... #}", Source: l.src}
	}
	trimRight := end > 0 && rem[end-1] == '-'
	bodyEnd := bodyStart + end
	if trimRight {
		bodyEnd--
	}
	body := l.src[bodyStart:bodyEnd]
	l.emit(token{Type: tokTwigCommentText, Lit: body, Raw: body, Pos: bodyPos})
	l.pt.advance(end)
	if trimRight {
		// Already consumed up to '-', now consume '-#}'
		// Wait: end is offset of '#}' within rem. So we've advanced to '#}' itself.
		// Re-think: we need to step back by 1 if trimRight, since body excluded the '-'.
		// Simpler approach: advance to the offset of '#}' minus the '-' if trimRight.
	}
	closePos := l.pt.pos()
	closeLen := 2
	closeStart := l.pt.cur.Offset
	// If trimRight, current position is at '#}', but body excluded preceding '-'.
	// Re-emit close as '-#}' (3 chars) backing up 1.
	if trimRight {
		closeStart--
		closeLen = 3
		closePos.Offset--
		closePos.Column--
	}
	l.emit(token{Type: tokTwigCommentClose, Lit: l.src[closeStart : closeStart+closeLen], Raw: l.src[closeStart : closeStart+closeLen], Pos: closePos, TrimRight: trimRight})
	// Advance past '#}' (we're already at '#}').
	l.pt.advance(2)
	return nil
}

// scanToTwigClose finds the offset of the closing delimiter (`%}` or `}}`),
// respecting string literals and bracket balance so values like
// `{% set x = "a%}b" %}` and `{{ x|filter({a: 1}) }}` parse correctly.
// Returns -1 if not found. trimRight is true when the closer is preceded by '-'.
func scanToTwigClose(src string, start int, close string) (int, bool, error) {
	depth := 0
	i := start
	for i < len(src) {
		// Check for close delimiter first when we're at bracket depth 0.
		if depth == 0 && i+1 < len(src) && src[i] == close[0] && src[i+1] == close[1] {
			if i > start && src[i-1] == '-' {
				return i - 1, true, nil
			}
			return i, false, nil
		}
		c := src[i]
		switch c {
		case '"', '\'':
			quote := c
			i++
			for i < len(src) && src[i] != quote {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				i++
			}
			if i < len(src) {
				i++
			}
		case '(', '[', '{':
			depth++
			i++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			i++
		default:
			i++
		}
	}
	return -1, false, nil
}
