package html

import "strings"

// TwigGenericBlockNode represents a Twig statement tag that opens a block,
// e.g. `{% for x in xs %}body{% endfor %}` or `{% embed 't' %}body{%
// endembed %}`. The body is parsed recursively as Twig+HTML content.
//
// This covers tags where there's no benefit to a dedicated AST type yet:
// the formatter just rebuilds `{% Name Args %}` + body + `{% EndTag %}`.
type TwigGenericBlockNode struct {
	Name   string
	Args   string // raw args body, with leading/trailing space stripped
	EndTag string
	Body   NodeList
	Line   int
}

func (n *TwigGenericBlockNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString("{% ")
	b.WriteString(n.Name)
	if n.Args != "" {
		b.WriteString(" ")
		b.WriteString(n.Args)
	}
	b.WriteString(" %}")

	if len(n.Body) > 0 {
		b.WriteString("\n")
		for i, child := range n.Body {
			if elem, ok := child.(*ElementNode); ok {
				b.WriteString(elem.Dump(indent + 1))
			} else {
				for j := 0; j < indent+1; j++ {
					b.WriteString(indentStr)
				}
				b.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
			}
			if i < len(n.Body)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		for i := 0; i < indent; i++ {
			b.WriteString(indentStr)
		}
	}

	b.WriteString("{% ")
	b.WriteString(n.EndTag)
	b.WriteString(" %}")
	return b.String()
}

// TwigStandaloneTagNode represents a Twig tag with no body, e.g.
// `{% include "x.twig" %}`, `{% extends "@base" %}`, `{% set x = 1 %}`,
// `{% sw_include "..." with {} %}`.
type TwigStandaloneTagNode struct {
	Name string
	Args string
	Line int
}

func (n *TwigStandaloneTagNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString("{% ")
	b.WriteString(n.Name)
	if n.Args != "" {
		b.WriteString(" ")
		b.WriteString(n.Args)
	}
	b.WriteString(" %}")
	return b.String()
}

// makeBlockTagParser returns a TagParseFunc that consumes
// `{% name args %}body{% endName %}` as a TwigGenericBlockNode.
func makeBlockTagParser(name, endTag string, followers []string) TagParseFunc {
	return func(p *parser, openTok token) (Node, error) {
		startLine := openTok.Pos.Line
		args, err := p.consumeStmtHeader(name)
		if err != nil {
			return nil, err
		}
		spec := lookupTag(name)
		body, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
		if err != nil {
			return nil, err
		}
		if reason != stopGenericEndTag {
			return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% %s %%}", endTag)
		}
		if err := p.consumeEndTag(endTag); err != nil {
			return nil, err
		}
		_ = followers
		return &TwigGenericBlockNode{
			Name:   name,
			Args:   args,
			EndTag: endTag,
			Body:   body,
			Line:   startLine,
		}, nil
	}
}

// makeStandaloneTagParser returns a TagParseFunc for `{% name args %}` with no body.
func makeStandaloneTagParser(name string) TagParseFunc {
	return func(p *parser, openTok token) (Node, error) {
		startLine := openTok.Pos.Line
		args, err := p.consumeStmtHeader(name)
		if err != nil {
			return nil, err
		}
		return &TwigStandaloneTagNode{
			Name: name,
			Args: args,
			Line: startLine,
		}, nil
	}
}
