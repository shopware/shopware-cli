package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// spanHeadingRenderer renders Markdown headings as <span class="hN"> elements
// instead of real <h1>-<h6> tags.
//
// The HTML produced by this package is pushed to the Shopware Store
// (extension description, installation manual and changelog) where it is
// embedded inside a store page that already owns its heading hierarchy
// (the <h1> is the product title). Emitting real headings for this description
// chrome would inject competing headings and harm the page's SEO structure, so
// we keep the visual hierarchy via CSS classes (class="h1" ... "h6") without the
// heading semantics, matching the Shopware Storefront SEO guideline.
type spanHeadingRenderer struct{}

func (r *spanHeadingRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
}

func (r *spanHeadingRenderer) renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)

	if entering {
		_, _ = w.WriteString(`<span class="h`)
		_ = w.WriteByte("0123456"[n.Level])
		_ = w.WriteByte('"')
		if n.Attributes() != nil {
			html.RenderAttributes(w, node, html.HeadingAttributeFilter)
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</span>\n")
	}

	return ast.WalkContinue, nil
}
