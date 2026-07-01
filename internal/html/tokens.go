package html

type tokenType int

const (
	tokEOF  tokenType = iota
	tokText           // raw text outside any tag/expression

	// HTML
	tokHTMLOpenStart  // '<' followed by tag name
	tokHTMLCloseStart // '</'
	tokHTMLTagName    // identifier after '<' or '</'
	tokHTMLTagEnd     // '>'
	tokHTMLSelfClose  // '/>'
	tokHTMLAttrName
	tokHTMLAttrEq // '='
	tokHTMLAttrValue
	tokHTMLComment // includes the full <!-- ... --> raw content (text accessor for body)
	tokHTMLDoctype // <!DOCTYPE ...>

	// Twig
	tokTwigStmtOpen  // {%  or {%-
	tokTwigStmtClose // %}  or -%}
	tokTwigExprOpen  // {{  or {{-
	tokTwigExprClose // }}  or -}}
	tokTwigCommentOpen
	tokTwigCommentClose
	tokTwigCommentText
	tokTwigIdent   // tag name or generic identifier
	tokTwigRawExpr // opaque body up to matching close delimiter
)

// token is the unit produced by the lexer.
//
// Lit and Raw are stored as [offset,len) windows into the source string rather
// than as string headers: every lexed literal is a substring of the source
// (even the whitespace-trimmed bodies, since strings.TrimSpace/TrimRight return
// subslices), so offsets lose no information. This keeps token pointer-free,
// which matters because the token stream is one large slice — a pointer-free
// element type is never scanned by the GC and its appends carry no write
// barriers, the two costs that dominated lexing once the buffer was right-sized.
// Recover the strings with the Lit(src)/Raw(src) accessors.
type token struct {
	Type   tokenType
	litOff int32
	litLen int32
	rawOff int32
	rawLen int32
	Pos    Pos
	// TrimLeft/TrimRight apply to twig delimiters: {%- / -%} / {{- / -}} / {#- / -#}
	TrimLeft  bool
	TrimRight bool
	// QuoteChar is set for tokHTMLAttrValue ('"', '\'', or 0 for bareword).
	QuoteChar byte
}

// Lit returns the literal text of the token. For tokHTMLAttrValue and
// tokHTMLComment it is the decoded/inner content; Raw is the verbatim slice.
func (t token) Lit(src string) string { return src[t.litOff : t.litOff+t.litLen] }

// Raw returns the verbatim source slice the token was scanned from.
func (t token) Raw(src string) string { return src[t.rawOff : t.rawOff+t.rawLen] }

// LitLen and RawLen return the byte lengths of Lit and Raw without needing the
// source (the common case where a caller only advances a buffer by the width).
func (t token) LitLen() int { return int(t.litLen) }
func (t token) RawLen() int { return int(t.rawLen) }
