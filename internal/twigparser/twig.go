package twigparser

import (
	"errors"
	"strings"
	"unicode"
)

const (
	tagTypeExpr  = "expr"
	tagTypeBlock = "block"
)

// tokenizeText splits a text string into a slice of Nodes. Each continuous
// segment of pure whitespace becomes a WhitespaceNode, whereas every other
// segment becomes a TextNode.
func tokenizeText(text string) NodeList {
	var nodes NodeList
	if len(text) == 0 {
		return nodes
	}

	var current strings.Builder
	// Determine the type for the first rune.
	firstRune, _ := utf8DecodeRuneInString(text)
	inWhitespace := isWhitespace(firstRune)

	for _, r := range text {
		if isWhitespace(r) == inWhitespace {
			current.WriteRune(r)
		} else {
			token := current.String()
			if inWhitespace {
				nodes = append(nodes, &WhitespaceNode{Text: token})
			} else {
				nodes = append(nodes, &TextNode{Text: token})
			}
			current.Reset()
			inWhitespace = isWhitespace(r)
			current.WriteRune(r)
		}
	}
	// Flush remaining token.
	if current.Len() > 0 {
		token := current.String()
		if inWhitespace {
			nodes = append(nodes, &WhitespaceNode{Text: token})
		} else {
			nodes = append(nodes, &TextNode{Text: token})
		}
	}
	return nodes
}

// utf8DecodeRuneInString is a helper to decode the first rune.
// If the string is empty, it returns 0.
func utf8DecodeRuneInString(s string) (r rune, size int) {
	if len(s) == 0 {
		return 0, 0
	}
	return []rune(s)[0], 1
}

// isWhitespace returns true if the rune is a whitespace character.
func isWhitespace(r rune) bool {
	return unicode.IsSpace(r)
}

// ParseTemplate is the entry point that builds an AST for the template.
func ParseTemplate(input string) (NodeList, error) {
	pos := 0
	return parseNodes(input, &pos, false)
}

// parseNodes walks through the input string from position *pos and returns
// a slice of nodes. It recognizes two types of tags:
//   - Block tags: delimited by {% ... %}.
//   - Expression tags: delimited by {{ ... }}.
//
// If stopOnEndBlock is true, parsing stops when a matching {% endblock %} is
// encountered.
// nolint: gocyclo
func parseNodes(input string, pos *int, stopOnEndBlock bool) ([]Node, error) {
	var nodes []Node

	for *pos < len(input) {
		relativeBlock := strings.Index(input[*pos:], "{%")
		relativeExpr := strings.Index(input[*pos:], "{{")
		var nextTagIndex int
		var tagType string

		switch {
		case relativeBlock == -1 && relativeExpr == -1:
			nextTagIndex = -1
		case relativeBlock == -1:
			nextTagIndex = relativeExpr
			tagType = tagTypeExpr
		case relativeExpr == -1:
			nextTagIndex = relativeBlock
			tagType = tagTypeBlock
		case relativeBlock < relativeExpr:
			nextTagIndex = relativeBlock
			tagType = tagTypeBlock
		default:
			nextTagIndex = relativeExpr
			tagType = tagTypeExpr
		}

		if nextTagIndex == -1 {
			remaining := input[*pos:]
			nodes = append(nodes, tokenizeText(remaining)...)
			*pos = len(input)
			break
		}

		tagStart := *pos + nextTagIndex
		if tagStart > *pos {
			text := input[*pos:tagStart]
			nodes = append(nodes, tokenizeText(text)...)
		}

		if tagType == tagTypeBlock {
			closeTagIndex := strings.Index(input[tagStart:], "%}")
			if closeTagIndex == -1 {
				return nil, errors.New("unclosed block tag")
			}
			tagEnd := tagStart + closeTagIndex + 2
			// Get tag content inside the delimiters.
			tagContent := strings.TrimSpace(input[tagStart+2 : tagStart+closeTagIndex])

			// Check for deprecated tag.
			if strings.HasPrefix(tagContent, "deprecated ") {
				message := strings.TrimSpace(tagContent[len("deprecated "):])
				// Remove surrounding quotes if present.
				message = strings.Trim(message, `"'`)
				nodes = append(nodes, &DeprecatedNode{Message: message})
				*pos = tagEnd
				continue
			}

			// Handle 'autoescape' tag.
			if strings.HasPrefix(tagContent, "autoescape") {
				// Optionally, support a custom strategy.
				strategy := "html"
				parts := strings.Fields(tagContent)
				if len(parts) > 1 {
					strategy = strings.Trim(parts[1], `"'`)
				}
				*pos = tagEnd
				children, err := parseNodes(input, pos, true)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, &AutoescapeNode{
					Strategy: strategy,
					Children: children,
				})
				continue
			} else if strings.HasPrefix(tagContent, "endautoescape") {
				if stopOnEndBlock {
					*pos = tagEnd
					return nodes, nil
				}
				// Treat unexpected end tag as literal text.
				nodes = append(nodes, tokenizeText(input[tagStart:tagEnd])...)
				*pos = tagEnd
				continue
			}

			// ADD: Handle 'set' tag.
			if strings.HasPrefix(tagContent, "set ") {
				assignment := strings.TrimSpace(tagContent[len("set "):])
				if strings.Contains(assignment, "=") {
					// Inline assignment.
					parts := strings.SplitN(assignment, "=", 2)
					lhs := strings.TrimSpace(parts[0])
					rhs := strings.TrimSpace(parts[1])
					varNames := splitAndTrim(lhs, ",")
					varValues := splitAndTrim(rhs, ",")
					nodes = append(nodes, &SetNode{
						Variables: varNames,
						Values:    varValues,
						IsBlock:   false,
					})
					*pos = tagEnd
					continue
				} else {
					// Block assignment.
					varNames := splitAndTrim(assignment, ",")
					*pos = tagEnd
					children, err := parseNodes(input, pos, true)
					if err != nil {
						return nil, err
					}
					nodes = append(nodes, &SetNode{
						Variables: varNames,
						IsBlock:   true,
						Children:  children,
					})
					continue
				}
			}

			// Handle 'types' tag.
			if strings.HasPrefix(tagContent, "types") {
				remainder := strings.TrimSpace(tagContent[len("types"):])
				typesNode, err := parseTypes(remainder)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, typesNode)
				*pos = tagEnd
				continue
			}

			// Handle block tags
			switch {
			case strings.HasPrefix(tagContent, "block "):
				parts := strings.Fields(tagContent)
				if len(parts) < 2 {
					return nil, errors.New("invalid block tag: no block name")
				}
				blockName := parts[1]
				*pos = tagEnd
				children, err := parseNodes(input, pos, true)
				if err != nil {
					return nil, err
				}
				block := &BlockNode{
					Name:     blockName,
					Children: children,
				}
				nodes = append(nodes, block)

			case strings.HasPrefix(tagContent, "sw_extends"):
				// New support for sw_extends.
				remainder := strings.TrimSpace(tagContent[len("sw_extends"):])
				var tmpl string
				scopes := []string{}
				if strings.HasPrefix(remainder, "{") {
					// Extended syntax: an object literal.
					startIdx := strings.Index(remainder, "{")
					endIdx := strings.LastIndex(remainder, "}")
					if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
						return nil, errors.New("invalid sw_extends syntax: missing or mismatched braces")
					}
					objectContent := strings.TrimSpace(remainder[startIdx+1 : endIdx])
					var err error
					tmpl, scopes, err = parseSwExtendsLiteral(objectContent)
					if err != nil {
						return nil, err
					}
				} else {
					// Simple syntax.
					parts := strings.Fields(tagContent)
					if len(parts) < 2 {
						return nil, errors.New("invalid sw_extends tag: missing template path")
					}
					tmpl = strings.Trim(parts[1], `"'`)
				}
				nodes = append(nodes, &SwExtendsNode{Template: tmpl, Scopes: scopes})
				*pos = tagEnd

			case strings.HasPrefix(tagContent, "endblock") || strings.HasPrefix(tagContent, "endfor"):
				if stopOnEndBlock {
					*pos = tagEnd
					return nodes, nil
				}
				// If an endblock appears unexpectedly, treat it as literal text.
				nodes = append(nodes, tokenizeText(input[tagStart:tagEnd])...)
				*pos = tagEnd

			default:
				// Unrecognized block tag: treat it as literal text.
				nodes = append(nodes, tokenizeText(input[tagStart:tagEnd])...)
				*pos = tagEnd
			}
			continue
		} else if tagType == tagTypeExpr {
			closeExprIndex := strings.Index(input[tagStart:], "}}")
			if closeExprIndex == -1 {
				return nil, errors.New("unclosed expression tag")
			}
			tagEnd := tagStart + closeExprIndex + 2
			exprContent := strings.TrimSpace(input[tagStart+2 : tagStart+closeExprIndex])
			if exprContent == "parent()" {
				nodes = append(nodes, &ParentNode{})
			} else {
				// Create a PrintNode for expressions like {{ a_variable }}
				nodes = append(nodes, &PrintNode{Expression: exprContent})
			}
			*pos = tagEnd
		}
	}

	return nodes, nil
}

// splitAndTrim splits the string s by the given sep and trims whitespace from each element.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var results []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

// parseSwExtendsLiteral parses the object literal inside a sw_extends tag.
// It expects an input like:
//
//	template: '@Storefront/storefront/page/checkout/finish/finish-details.html.twig',
//	scopes: ['default', 'subscription']
func parseSwExtendsLiteral(s string) (string, []string, error) {
	var tmpl string
	var scopes []string

	// Parse the "template:" value.
	templateIdx := strings.Index(s, "template:")
	if templateIdx == -1 {
		return "", nil, errors.New("sw_extends literal missing 'template' key")
	}
	rest := s[templateIdx+len("template:"):]
	// Find the end of the template value (either a comma or end-of-string).
	endIdx := strings.Index(rest, ",")
	var tmplValue string
	if endIdx == -1 {
		tmplValue = rest
	} else {
		tmplValue = rest[:endIdx]
	}
	tmpl = strings.TrimSpace(tmplValue)
	tmpl = strings.Trim(tmpl, `"'`)

	// Look for the "scopes:" key.
	scopesIdx := strings.Index(s, "scopes:")
	if scopesIdx != -1 {
		rest = s[scopesIdx+len("scopes:"):]
		rest = strings.TrimSpace(rest)
		if len(rest) > 0 && rest[0] == '[' {
			// Find the closing bracket.
			endArrIdx := strings.Index(rest, "]")
			if endArrIdx == -1 {
				return "", nil, errors.New("invalid scopes array: missing ']'")
			}
			arrContent := rest[1:endArrIdx]
			// Split by commas.
			parts := strings.Split(arrContent, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				p = strings.Trim(p, `"'`)
				if p != "" {
					scopes = append(scopes, p)
				}
			}
		}
	}
	return tmpl, scopes, nil
}
