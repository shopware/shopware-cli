package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// storeHeadingRenderer renders Markdown headings as semantic heading
// elements clamped to the <h2>-<h4> range.
//
// The HTML produced by this package is pushed to the Shopware Store
// (extension description, installation manual and changelog). The Store
// only allows <h2>, <h3> and <h4> headings: the store page already owns
// the <h1> (the product title), and <h5>/<h6> are rejected just like any
// other disallowed tag. Markdown headings outside the allowed range are
// therefore clamped to the nearest allowed level instead of being emitted
// verbatim (or as <span> elements, which the Store also rejects).
type storeHeadingRenderer struct{}

func (r *storeHeadingRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.renderHeading)
}

func (r *storeHeadingRenderer) renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)

	level := min(max(n.Level, 2), 4)

	if entering {
		_, _ = w.WriteString("<h")
		_ = w.WriteByte("0123456"[level])
		if n.Attributes() != nil {
			html.RenderAttributes(w, node, html.HeadingAttributeFilter)
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</h")
		_ = w.WriteByte("0123456"[level])
		_, _ = w.WriteString(">\n")
	}

	return ast.WalkContinue, nil
}
