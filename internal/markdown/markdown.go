// Package markdown renders Markdown to HTML for content that is pushed to the
// Shopware Store (extension description, installation manual and changelog).
//
// The generated HTML is embedded inside a store page, and the Store only
// allows <h2>-<h4> headings, so the renderer clamps Markdown headings to
// that range (see heading_renderer.go).
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	goldmarkExtension "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// New returns a goldmark instance configured for rendering Markdown that is
// pushed to the Shopware Store.
func New() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(goldmarkExtension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			// Override heading rendering so Markdown headings are clamped
			// to the <h2>-<h4> range allowed by the Store. Priority < 1000
			// makes it win over the default HTML renderer.
			renderer.WithNodeRenderers(util.Prioritized(&storeHeadingRenderer{}, 100)),
		),
	)
}

// ToHTML converts Markdown content to HTML using the store-configured renderer.
func ToHTML(content []byte) (string, error) {
	var buf bytes.Buffer
	if err := New().Convert(content, &buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}
