package html

import "strings"

// TwigCommentNode represents a `{# ... #}` Twig comment. The body is the
// raw text between the delimiters, preserved verbatim (including leading
// and trailing whitespace).
type TwigCommentNode struct {
	Body string
	Trim TwigTrim
	Line int
}

func (c *TwigCommentNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString(openComment(c.Trim.Left))
	b.WriteString(normalizeTwigCommentBody(c.Body))
	b.WriteString(closeComment(c.Trim.Right))
	return b.String()
}

func normalizeTwigCommentBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return body
	}
	return " " + trimmed + " "
}
