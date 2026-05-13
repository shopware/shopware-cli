package html

import "strings"

// TwigCommentNode represents a `{# ... #}` Twig comment. The body is the
// raw text between the delimiters, preserved verbatim (including leading
// and trailing whitespace).
type TwigCommentNode struct {
	Body string
	Line int
}

func (c *TwigCommentNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString("{#")
	b.WriteString(c.Body)
	b.WriteString("#}")
	return b.String()
}
