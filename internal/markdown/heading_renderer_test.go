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

func TestMarkdownHeadings(t *testing.T) {
	html := renderMarkdown(t, "## Title\n\n### Subtitle\n\nText")

	assert.Contains(t, html, `<h2>Title</h2>`)
	assert.Contains(t, html, `<h3>Subtitle</h3>`)
	assert.NotContains(t, html, "<span")
}

func TestMarkdownHeadingLevelsClamped(t *testing.T) {
	html := renderMarkdown(t, "# A\n\n## B\n\n### C\n\n#### D\n\n##### E\n\n###### F")

	// The Store only allows <h2>-<h4>; <h1> is the store page's own title.
	assert.Contains(t, html, `<h2>A</h2>`)
	assert.Contains(t, html, `<h2>B</h2>`)
	assert.Contains(t, html, `<h3>C</h3>`)
	assert.Contains(t, html, `<h4>D</h4>`)
	assert.Contains(t, html, `<h4>E</h4>`)
	assert.Contains(t, html, `<h4>F</h4>`)
	assert.NotContains(t, html, "<h1")
	assert.NotContains(t, html, "<h5")
	assert.NotContains(t, html, "<h6")
	assert.NotContains(t, html, "<span")
}

func TestMarkdownNonHeadingContentUnchanged(t *testing.T) {
	html := renderMarkdown(t, "Some **bold** text with a [link](https://example.com).")

	assert.Contains(t, html, "<strong>bold</strong>")
	assert.Contains(t, html, `<a href="https://example.com">link</a>`)
}
