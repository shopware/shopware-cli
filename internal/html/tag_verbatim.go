package html

import "strings"

func init() {
	registerTag(TagSpec{
		Name:   "verbatim",
		EndTag: "endverbatim",
		Parse:  parseVerbatimTag,
	})
}

// TwigVerbatimNode represents `{% verbatim %}...{% endverbatim %}`.
// The body is preserved as raw text — the parser does NOT recurse into
// the body, since the whole point of `{% verbatim %}` is to disable Twig
// interpretation for its contents.
type TwigVerbatimNode struct {
	Body string
	Line int
}

// Dump renders the verbatim block with its body byte-identical to source.
func (v *TwigVerbatimNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString("{% verbatim %}")
	b.WriteString(v.Body)
	b.WriteString("{% endverbatim %}")
	return b.String()
}

// parseVerbatimTag consumes `{% verbatim %}body{% endverbatim %}` and stores
// the body verbatim. It bypasses the registry's normal recursion so Twig
// constructs inside the body are not re-parsed.
func parseVerbatimTag(p *parser, openTok token) (Node, error) {
	startLine := openTok.Pos.Line
	if _, err := p.consumeStmtHeader("verbatim"); err != nil {
		return nil, err
	}

	// Sweep tokens until we hit `{% endverbatim %}`. Reassemble the body
	// from each token's Raw so whitespace, expressions, and even nested
	// {% if %} etc. inside verbatim round-trip exactly.
	var body strings.Builder
	for {
		tk := p.peek(0)
		if tk.Type == tokEOF {
			return nil, errAt(p.source, p.filename, openTok.Pos, "unterminated {%% verbatim %%}")
		}
		if tk.Type == tokTwigStmtOpen {
			identTok := p.peek(1)
			if identTok.Type == tokTwigIdent && identTok.Lit == "endverbatim" {
				break
			}
		}
		body.WriteString(tk.Raw)
		p.advance()
	}
	if err := p.consumeEndTag("endverbatim"); err != nil {
		return nil, err
	}
	return &TwigVerbatimNode{Body: body.String(), Line: startLine}, nil
}
