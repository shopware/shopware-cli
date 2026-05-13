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

	condition, err := p.consumeStmtHeader("if")
	if err != nil {
		return nil, err
	}

	spec := lookupTag("if")
	ifBody, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
	if err != nil {
		return nil, err
	}

	branches := []TwigIfBranch{{Condition: condition, Body: ifBody}}
	var elseChildren NodeList

	// Walk follower tags (elseif... else) then expect endif.
	for reason == stopIfTerminator {
		nameTok := p.peek(1)
		if nameTok.Type != tokTwigIdent {
			return nil, errAt(p.source, p.filename, nameTok.Pos, "expected if-follower identifier")
		}
		switch nameTok.Lit {
		case "elseif":
			cond, err := p.consumeStmtHeader("elseif")
			if err != nil {
				return nil, err
			}
			body, r, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
			if err != nil {
				return nil, err
			}
			branches = append(branches, TwigIfBranch{Condition: cond, Body: body})
			reason = r
		case "else":
			if _, err := p.consumeStmtHeader("else"); err != nil {
				return nil, err
			}
			body, r, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
			if err != nil {
				return nil, err
			}
			elseChildren = body
			reason = r
		default:
			return nil, errAt(p.source, p.filename, nameTok.Pos, "unexpected if-follower %q", nameTok.Lit)
		}
	}

	if reason != stopGenericEndTag {
		return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% endif %%}")
	}
	if err := p.consumeEndTag("endif"); err != nil {
		return nil, err
	}

	return &TwigIfNode{
		Branches:     branches,
		ElseChildren: elseChildren,
		Line:         startLine,
	}, nil
}

// consumeStmtHeader consumes `{% name body %}` and returns the trimmed body.
func (p *parser) consumeStmtHeader(name string) (string, error) {
	openTok := p.peek(0)
	if openTok.Type != tokTwigStmtOpen {
		return "", errAt(p.source, p.filename, openTok.Pos, "expected '{%%' for '%s'", name)
	}
	p.advance()
	identTok := p.advance()
	if identTok.Type != tokTwigIdent || identTok.Lit != name {
		return "", errAt(p.source, p.filename, identTok.Pos, "expected '%s'", name)
	}
	body := ""
	if p.peek(0).Type == tokTwigRawExpr {
		body = strings.TrimSpace(p.advance().Lit)
	}
	closeTok := p.peek(0)
	if closeTok.Type != tokTwigStmtClose {
		return "", errAt(p.source, p.filename, closeTok.Pos, "expected '%%}' to close '%s'", name)
	}
	p.advance()
	return body, nil
}
