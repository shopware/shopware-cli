package html

import "strings"

func init() {
	registerTag(TagSpec{
		Name:   "set",
		EndTag: "endset", // honored only for the block form
		Parse:  parseSetTag,
	})
}

// parseSetTag handles both forms:
//
//	{% set x = expr %}                    — inline, no body, no endset
//	{% set x %}body{% endset %}           — block, has endset
func parseSetTag(p *parser, openTok token) (Node, error) {
	startLine := openTok.Pos.Line
	args, openTrim, err := p.consumeStmtHeader("set")
	if err != nil {
		return nil, err
	}
	// Inline form: args contain `=` outside any string literal. The simple
	// substring check is adequate because Twig's `==` would also match,
	// and `==` only appears in expression contexts where `set` itself
	// already implies the inline form.
	if strings.Contains(args, "=") {
		return &TwigStandaloneTagNode{Name: "set", Args: args, Trim: openTrim, Line: startLine}, nil
	}
	spec := lookupTag("set")
	body, reason, err := p.parseNodesUntil(nodeContextTopLevel, "", spec)
	if err != nil {
		return nil, err
	}
	if reason != stopGenericEndTag {
		return nil, errAt(p.source, p.filename, openTok.Pos, "missing {%% endset %%}")
	}
	closeTrim, err := p.consumeEndTag("endset")
	if err != nil {
		return nil, err
	}
	return &TwigGenericBlockNode{
		Name:      "set",
		Args:      args,
		EndTag:    "endset",
		Body:      body,
		OpenTrim:  openTrim,
		CloseTrim: closeTrim,
		Line:      startLine,
	}, nil
}
