package html

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCustomTagRegistration exercises the public Register* API by
// registering a project-specific block tag and verifying that the parser
// builds a TwigGenericBlockNode for it (rather than falling back to raw
// text). Format/round-trip behavior for registered tags is covered by
// the testdata fixtures.
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

// TestParentRejectsArguments verifies that the parent-tag parser rejects
// unexpected arguments — silently rewriting `{% parent foo %}` as
// `{% parent() %}` would hide real authoring mistakes. The accepted forms
// (`{% parent %}`, `{% parent() %}`, `{%- parent -%}`) have their format
// round-trip pinned by testdata/53-trim-parent.txt and other fixtures.
func TestParentRejectsArguments(t *testing.T) {
	bad := []string{
		`{% parent foo %}`,
		`{% parent 1 %}`,
		`{% parent(x) %}`,
	}
	for _, src := range bad {
		t.Run(src, func(t *testing.T) {
			_, err := NewParser(src)
			assert.Error(t, err, "parent with unexpected argument must not silently parse")
		})
	}
}

// TestForLoopAstShape pins that {% for %}{% else %}{% endfor %} produces a
// TwigGenericBlockNode with both Body and Else populated. The byte-level
// format round-trip is pinned by testdata/57-for-else-loop.txt.
func TestForLoopAstShape(t *testing.T) {
	src := "{% for item in items %}\n    item: {{ item }}\n{% else %}\n    empty\n{% endfor %}"
	nodes, err := NewParser(src)
	assert.NoError(t, err)

	var loop *TwigGenericBlockNode
	for _, n := range nodes {
		if g, ok := n.(*TwigGenericBlockNode); ok && g.Name == "for" {
			loop = g
			break
		}
	}
	if !assert.NotNil(t, loop, "for-loop should parse as TwigGenericBlockNode") {
		return
	}
	assert.NotEmpty(t, loop.Body, "for body should not be empty")
	assert.NotEmpty(t, loop.Else, "for else branch should not be empty")
	assert.Contains(t, loop.Args, "items")
}
