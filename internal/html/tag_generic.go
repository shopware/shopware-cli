package html

import "strings"

// TwigGenericBlockNode represents a Twig statement tag that opens a block,
// e.g. `{% for x in xs %}body{% endfor %}` or `{% embed 't' %}body{%
// endembed %}`. The body is parsed recursively as Twig+HTML content.
//
// Else is populated when the block tag supports an `{% else %}` follower
// (currently just `{% for %}`); it holds the body between `{% else %}` and
// the block's EndTag. Else is nil/empty for tags without an else clause.
type TwigGenericBlockNode struct {
	Name      string
	Args      string // raw args body, with leading/trailing space stripped
	EndTag    string
	Body      NodeList
	Else      NodeList
	OpenTrim  TwigTrim
	ElseTrim  TwigTrim
	CloseTrim TwigTrim
	Line      int
}

func (n *TwigGenericBlockNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString(openStmt(n.OpenTrim.Left))
	b.WriteString(" ")
	b.WriteString(n.Name)
	if n.Args != "" {
		b.WriteString(" ")
		b.WriteString(n.Args)
	}
	b.WriteString(" ")
	b.WriteString(closeStmt(n.OpenTrim.Right))

	// Inline-mixed body (text + {{ x }} only, no nested blocks/elements):
	// flow children verbatim so embedded whitespace drives layout. Without
	// this, the per-child re-indent and TrimSpace strip the spaces around
	// expressions and the layout drifts on every format pass. We never take
	// the inline-mixed path when there's an `{% else %}` branch to render —
	// the else clause needs the structured block layout.
	if len(n.Else) == 0 && blockHasInlineMixedContent(n.Body) {
		for _, child := range n.Body {
			if _, ok := child.(*TwigCommentNode); ok {
				b.WriteString(child.Dump(0))
				continue
			}
			b.WriteString(child.Dump(indent))
		}
		b.WriteString(openStmt(n.CloseTrim.Left))
		b.WriteString(" ")
		b.WriteString(n.EndTag)
		b.WriteString(" ")
		b.WriteString(closeStmt(n.CloseTrim.Right))
		return b.String()
	}

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

	if len(n.Else) > 0 {
		b.WriteString(openStmt(n.ElseTrim.Left))
		b.WriteString(" else ")
		b.WriteString(closeStmt(n.ElseTrim.Right))
		b.WriteString("\n")
		for i, child := range n.Else {
			if elem, ok := child.(*ElementNode); ok {
				b.WriteString(elem.Dump(indent + 1))
			} else {
				for j := 0; j < indent+1; j++ {
					b.WriteString(indentStr)
				}
				b.WriteString(strings.TrimSpace(child.Dump(indent + 1)))
			}
			if i < len(n.Else)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		for i := 0; i < indent; i++ {
			b.WriteString(indentStr)
		}
	}

	b.WriteString(openStmt(n.CloseTrim.Left))
	b.WriteString(" ")
	b.WriteString(n.EndTag)
	b.WriteString(" ")
	b.WriteString(closeStmt(n.CloseTrim.Right))
	return b.String()
}

// TwigStandaloneTagNode represents a Twig tag with no body, e.g.
// `{% include "x.twig" %}`, `{% extends "@base" %}`, `{% set x = 1 %}`,
// `{% sw_include "..." with {} %}`.
type TwigStandaloneTagNode struct {
	Name string
	Args string
	Trim TwigTrim
	Line int
}

func (n *TwigStandaloneTagNode) Dump(indent int) string {
	var b strings.Builder
	indentStr := indentConfig.GetIndent()
	for i := 0; i < indent; i++ {
		b.WriteString(indentStr)
	}
	b.WriteString(openStmt(n.Trim.Left))
	b.WriteString(" ")
	b.WriteString(n.Name)
	if n.Args != "" {
		b.WriteString(" ")
		b.WriteString(n.Args)
	}
	b.WriteString(" ")
	b.WriteString(closeStmt(n.Trim.Right))
	return b.String()
}

// makeBlockTagParser returns a TagParseFunc that consumes
// `{% name args %}body{% endName %}` as a TwigGenericBlockNode. If
// followers contains "else" (the only follower currently supported for
// generic block tags — used by `{% for %}`), it also consumes
// `{% else %}elseBody` before the end tag and stores it on Node.Else.
func makeBlockTagParser(name, endTag string, followers []string) TagParseFunc {
	supportsElse := false
	for _, f := range followers {
		if f == "else" {
			supportsElse = true
			break
		}
	}
	return func(p *parser, openTok token) (Node, error) {
		startLine := openTok.Pos.Line
		args, openTrim, err := p.consumeStmtHeader(name)
		if err != nil {
			return nil, err
		}
		spec := lookupTag(name)
		body, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
		if err != nil {
			return nil, err
		}
		var elseBody NodeList
		var elseTrim TwigTrim
		// If parseNodesUntil yielded on a follower (e.g. `{% else %}`),
		// consume it and parse the alternate branch before expecting the
		// end tag. Today only `{% else %}` is supported as a follower for
		// generic block tags — `{% for %}` is the only built-in user.
		if reason == stopIfTerminator && supportsElse {
			nameTok := p.peek(1)
			if nameTok.Type != tokTwigIdent || nameTok.Lit(p.source) != "else" {
				return nil, errAt(p.source, p.filename, nameTok.Pos, "expected {%% else %%} inside {%% %s %%}", name)
			}
			_, t, err := p.consumeStmtHeader("else")
			if err != nil {
				return nil, err
			}
			elseTrim = t
			elseBody, reason, err = p.parseNodesUntil(nodeContextTopLevel, "", spec)
			if err != nil {
				return nil, err
			}
		}
		if reason != stopGenericEndTag {
			return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% %s %%}", endTag)
		}
		closeTrim, err := p.consumeEndTag(endTag)
		if err != nil {
			return nil, err
		}
		return &TwigGenericBlockNode{
			Name:      name,
			Args:      args,
			EndTag:    endTag,
			Body:      body,
			Else:      elseBody,
			OpenTrim:  openTrim,
			ElseTrim:  elseTrim,
			CloseTrim: closeTrim,
			Line:      startLine,
		}, nil
	}
}

// makeStandaloneTagParser returns a TagParseFunc for `{% name args %}` with no body.
func makeStandaloneTagParser(name string) TagParseFunc {
	return func(p *parser, openTok token) (Node, error) {
		startLine := openTok.Pos.Line
		args, trim, err := p.consumeStmtHeader(name)
		if err != nil {
			return nil, err
		}
		return &TwigStandaloneTagNode{
			Name: name,
			Args: args,
			Trim: trim,
			Line: startLine,
		}, nil
	}
}
