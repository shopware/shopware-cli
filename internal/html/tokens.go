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
// Lit and Raw are stored as [offset,len) windows into the source rather than as
// strings: every lexed literal is a substring of the source (trimmed bodies
// included, since strings.TrimSpace/TrimRight return subslices), so no
// information is lost. Keeping token pointer-free means the token slice is not
// GC-scanned and appends need no write barriers. Use the Lit(src)/Raw(src)
// accessors to recover the strings.
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

// LitLen and RawLen return the byte lengths of Lit and Raw without the source.
func (t token) LitLen() int { return int(t.litLen) }
func (t token) RawLen() int { return int(t.rawLen) }
