package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func renderMarkdown(t *testing.T, source string) string {
	t.Helper()

	html, err := ToHTML([]byte(source))
	assert.NoError(t, err)

	return html
}

func TestMarkdownHeadingsRenderedAsSpans(t *testing.T) {
	html := renderMarkdown(t, "# Title\n\n## Subtitle\n\nText")

	// Headings must not emit real <h1>-<h6> tags so they do not compete with
	// the store page heading hierarchy.
	assert.NotContains(t, html, "<h1")
	assert.NotContains(t, html, "<h2")
	assert.Contains(t, html, `<span class="h1"`)
	assert.Contains(t, html, `<span class="h2"`)
	assert.Contains(t, html, "Title</span>")
	assert.Contains(t, html, "Subtitle</span>")
}

func TestMarkdownAllHeadingLevelsRenderedAsSpans(t *testing.T) {
	html := renderMarkdown(t, "# A\n\n## B\n\n### C\n\n#### D\n\n##### E\n\n###### F")

	for _, level := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		assert.Contains(t, html, `<span class="`+level+`"`)
		assert.NotContains(t, html, "<"+level)
	}
}

func TestMarkdownNonHeadingContentUnchanged(t *testing.T) {
	html := renderMarkdown(t, "Some **bold** text with a [link](https://example.com).")

	assert.Contains(t, html, "<strong>bold</strong>")
	assert.Contains(t, html, `<a href="https://example.com">link</a>`)
}
