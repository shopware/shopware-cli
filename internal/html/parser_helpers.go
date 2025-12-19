package html

// parseTwigNodesInElement attempts to parse various Twig structures
func (p *Parser) parseTwigNodesInElement() (Node, error) {
	startPos := p.pos

	// Try parsing as a twig directive first
	directive, err := p.parseTwigDirective()
	if err != nil {
		return nil, err
	}
	if directive != nil {
		return directive, nil
	}
	p.pos = startPos

	// Try parsing as a twig block
	block, err := p.parseTwigBlock()
	if err != nil {
		return nil, err
	}
	if block != nil {
		return block, nil
	}
	p.pos = startPos

	// Try parsing as a twig if
	ifNode, err := p.parseTwigIf()
	if err != nil {
		return nil, err
	}
	if ifNode != nil {
		return ifNode, nil
	}
	p.pos = startPos

	return nil, nil //nolint:nilnil
}

// flushRawText appends raw text to the children list if any exists
func (p *Parser) flushRawText(children NodeList, start, end int) NodeList {
	if end > start {
		text := p.input[start:end]
		if text != "" {
			return append(children, &RawNode{
				Text: text,
				Line: p.getLineAt(start),
			})
		}
	}
	return children
}
