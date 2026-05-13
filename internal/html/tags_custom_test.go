package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCustomTagRegistration exercises the public Register* API by
// registering a project-specific block tag and verifying that the parser
// builds a TwigGenericBlockNode for it (rather than falling back to raw
// text) and that the formatter round-trips it cleanly.
func TestCustomTagRegistration(t *testing.T) {
	// A made-up tag for the test; this name is unlikely to collide with a
	// real Twig or Shopware tag now or in future. If you see a panic from
	// "duplicate tag registration" here, rename the tag.
	const tagName = "custom_html_test_block"
	const endTag = "endcustom_html_test_block"

	RegisterBlockTag(tagName, endTag)

	src := "{% " + tagName + " 'arg' %}\n    hello {{ name }}\n{% " + endTag + " %}"

	nodes, err := NewParser(src)
	assert.NoError(t, err)

	// Walk the AST: expect a TwigGenericBlockNode at the top level.
	var found *TwigGenericBlockNode
	for _, n := range nodes {
		if g, ok := n.(*TwigGenericBlockNode); ok && g.Name == tagName {
			found = g
			break
		}
	}
	if !assert.NotNil(t, found, "registered custom tag should produce a TwigGenericBlockNode") {
		return
	}
	assert.Equal(t, "'arg'", found.Args)
	assert.Equal(t, endTag, found.EndTag)
	assert.NotEmpty(t, found.Body)

	// Round-trip the formatted output.
	out := nodes.Dump(0)
	assert.True(t, strings.Contains(out, "{% "+tagName))
	assert.True(t, strings.Contains(out, "{% "+endTag))

	reparsed, err := NewParser(out)
	assert.NoError(t, err)
	assert.Equal(t, out, reparsed.Dump(0), "custom block tag must format idempotently")
}

// TestCustomStandaloneTagRegistration exercises RegisterStandaloneTag.
func TestCustomStandaloneTagRegistration(t *testing.T) {
	const tagName = "custom_html_test_standalone"
	RegisterStandaloneTag(tagName)

	src := "{% " + tagName + " 'arg' %}"
	nodes, err := NewParser(src)
	assert.NoError(t, err)

	var found *TwigStandaloneTagNode
	for _, n := range nodes {
		if s, ok := n.(*TwigStandaloneTagNode); ok && s.Name == tagName {
			found = s
			break
		}
	}
	if !assert.NotNil(t, found, "registered standalone tag should produce a TwigStandaloneTagNode") {
		return
	}
	assert.Equal(t, "'arg'", found.Args)
}

// TestUnregisteredTagFallback documents that standalone tags work out of
// the box via raw-text round-trip even without registration.
func TestUnregisteredTagFallback(t *testing.T) {
	src := "{% completely_unknown_tag 'a' %}"
	nodes, err := NewParser(src)
	assert.NoError(t, err)
	// No structured node — the bytes live as a RawNode.
	assert.Equal(t, src, nodes.Dump(0))
}
