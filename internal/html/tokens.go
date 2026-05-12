package html

type tokenType int

const (
	tokEOF tokenType = iota
	tokText // raw text outside any tag/expression

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
type token struct {
	Type tokenType
	// Lit is the literal text from source. For tokHTMLAttrValue and tokHTMLComment
	// it carries the decoded/inner content; Raw covers the verbatim slice.
	Lit string
	Raw string
	Pos Pos
	// TrimLeft/TrimRight apply to twig delimiters: {%- / -%} / {{- / -}} / {#- / -#}
	TrimLeft  bool
	TrimRight bool
	// QuoteChar is set for tokHTMLAttrValue ('"', '\'', or 0 for bareword).
	QuoteChar byte
}
