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

func TestMarkdownHeadingsRenderedAsSemanticHeadings(t *testing.T) {
	html := renderMarkdown(t, "## Digitale Inhalte professionell bereitstellen\n\n### Details\n\nText")

	assert.Contains(t, html, `<h2 id="digitale-inhalte-professionell-bereitstellen">Digitale Inhalte professionell bereitstellen</h2>`)
	assert.Contains(t, html, `<h3 id="details">Details</h3>`)
	assert.NotContains(t, html, "<span")
}

func TestMarkdownHeadingLevelsClampedToStoreRange(t *testing.T) {
	html := renderMarkdown(t, "# A\n\n## B\n\n### C\n\n#### D\n\n##### E\n\n###### F")

	// The Store only allows <h2>-<h4>: <h1> belongs to the store page
	// itself and <h5>/<h6> are rejected.
	assert.Contains(t, html, `<h2 id="a">A</h2>`)
	assert.Contains(t, html, `<h2 id="b">B</h2>`)
	assert.Contains(t, html, `<h3 id="c">C</h3>`)
	assert.Contains(t, html, `<h4 id="d">D</h4>`)
	assert.Contains(t, html, `<h4 id="e">E</h4>`)
	assert.Contains(t, html, `<h4 id="f">F</h4>`)
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
