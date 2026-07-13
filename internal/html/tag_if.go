package html

import "strings"

func init() {
	registerTag(TagSpec{
		Name:      "if",
		EndTag:    "endif",
		Followers: []string{"elseif", "else"},
		Parse:     parseIfTag,
	})
}

// parseIfTag parses `{% if cond %}...{% elseif x %}...{% else %}...{% endif %}`
// into a TwigIfNode whose Branches slice holds the "if" entry plus every
// "elseif" in order. The else clause (if any) lives on ElseChildren.
func parseIfTag(p *parser, openTok token) (Node, error) {
	if openTok.Type != tokTwigStmtOpen {
		return nil, errAt(p.source, p.filename, openTok.Pos, "expected open delimiter for if tag")
	}
	startLine := openTok.Pos.Line

	condition, ifTrim, err := p.consumeStmtHeader("if")
	if err != nil {
		return nil, err
	}

	spec := lookupTag("if")
	ifBody, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
	if err != nil {
		return nil, err
	}

	branches := []TwigIfBranch{{Condition: condition, Body: ifBody, Trim: ifTrim}}
	var elseChildren NodeList
	var elseTrim TwigTrim

	// Walk follower tags (elseif... else) then expect endif.
	for reason == stopIfTerminator {
		nameTok := p.peek(1)
		if nameTok.Type != tokTwigIdent {
			return nil, errAt(p.source, p.filename, nameTok.Pos, "expected if-follower identifier")
		}
		switch nameTok.Lit(p.source) {
		case "elseif":
			cond, trim, err := p.consumeStmtHeader("elseif")
			if err != nil {
				return nil, err
			}
			body, r, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
			if err != nil {
				return nil, err
			}
			branches = append(branches, TwigIfBranch{Condition: cond, Body: body, Trim: trim})
			reason = r
		case "else":
			_, trim, err := p.consumeStmtHeader("else")
			if err != nil {
				return nil, err
			}
			elseTrim = trim
			body, r, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
			if err != nil {
				return nil, err
			}
			elseChildren = body
			reason = r
		default:
			return nil, errAt(p.source, p.filename, nameTok.Pos, "unexpected if-follower %q", nameTok.Lit(p.source))
		}
	}

	if reason != stopGenericEndTag {
		return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% endif %%}")
	}
	endTrim, err := p.consumeEndTag("endif")
	if err != nil {
		return nil, err
	}

	return &TwigIfNode{
		Branches:     branches,
		ElseChildren: elseChildren,
		ElseTrim:     elseTrim,
		EndTrim:      endTrim,
		Line:         startLine,
	}, nil
}

// consumeStmtHeader consumes `{% name body %}` and returns the trimmed body
// plus the trim flags on the open/close delimiters.
func (p *parser) consumeStmtHeader(name string) (string, TwigTrim, error) {
	openTok := p.peek(0)
	if openTok.Type != tokTwigStmtOpen {
		return "", TwigTrim{}, errAt(p.source, p.filename, openTok.Pos, "expected '{%%' for '%s'", name)
	}
	trim := TwigTrim{Left: openTok.TrimLeft}
	p.advance()
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit(p.source) != name {
		return "", TwigTrim{}, errAt(p.source, p.filename, identTok.Pos, "expected '%s'", name)
	}
	body := ""
	if p.peek(0).Type == tokTwigRawExpr {
		body = strings.TrimSpace(p.advance().Lit(p.source))
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return "", TwigTrim{}, errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' to close '%s'", name)
	}
	trim.Right = closeTok.TrimRight
	p.advance()
	return body, trim, nil
}
