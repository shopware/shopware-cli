package markdown

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// storeHeadingRenderer renders Markdown headings as <h2>-<h4> elements, as
// the Shopware Store rejects other heading levels and <span> elements.
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
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</h")
		_ = w.WriteByte("0123456"[level])
		_, _ = w.WriteString(">\n")
	}

	return ast.WalkContinue, nil
}
