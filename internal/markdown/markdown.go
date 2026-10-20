// Package markdown renders Markdown to HTML for content that is pushed to the
// Shopware Store (extension description, installation manual and changelog).
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	goldmarkExtension "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// New returns a goldmark instance configured for rendering Markdown that is
// pushed to the Shopware Store.
func New() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(goldmarkExtension.GFM),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			// Clamp headings to the Store's <h2>-<h4> range; a priority below
			// 1000 wins over the default HTML renderer.
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
