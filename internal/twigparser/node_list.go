package twigparser

import "strings"

type NodeList []Node

func (nl NodeList) Find(predicate func(Node) bool) NodeList {
	var result NodeList
	for _, node := range nl {
		if predicate(node) {
			result = append(result, node)
		}
		// If the node is a BlockNode, search recursively in its children.
		if block, ok := node.(*BlockNode); ok {
			nestedMatches := block.Children.Find(predicate)
			result = append(result, nestedMatches...)
		}
	}
	return result
}

func (nl NodeList) FindBlock(name string) *BlockNode {
	matches := nl.Find(func(node Node) bool {
		block, ok := node.(*BlockNode)
		return ok && block.Name == name
	})
	if len(matches) == 0 {
		return nil
	}
	return matches[0].(*BlockNode)
}

func (nl NodeList) Extends() *SwExtendsNode {
	matches := nl.Find(func(node Node) bool {
		_, ok := node.(*SwExtendsNode)
		return ok
	})

	if len(matches) == 0 {
		return nil
	}

	return matches[0].(*SwExtendsNode)
}

func (nl NodeList) BlockNames() []string {
	matches := nl.Find(func(node Node) bool {
		_, ok := node.(*BlockNode)
		return ok
	})

	var result []string

	for _, node := range matches {
		result = append(result, node.(*BlockNode).Name)
	}

	return result
}

func (nl NodeList) Traverse(visitor func(Node) Node) NodeList {
	for i, node := range nl {
		// If the node has children, traverse them first.
		if block, ok := node.(*BlockNode); ok {
			block.Children = block.Children.Traverse(visitor)
		}
		// Apply the visitor function.
		nl[i] = visitor(node)
	}
	return nl
}

func (nl NodeList) RemoveWhitespace() NodeList {
	return nl.Find(func(node Node) bool {
		_, isWhitespace := node.(*WhitespaceNode)
		return !isWhitespace
	})
}

func (nl NodeList) String() string {
	var sb strings.Builder

	for _, node := range nl {
		sb.WriteString(node.String(""))
	}

	return sb.String()
}

func (nl NodeList) Dump() string {
	var sb strings.Builder
	for _, node := range nl {
		sb.WriteString(node.Dump())
	}
	return sb.String()
}
