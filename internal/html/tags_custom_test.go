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

// TestParentRejectsArguments is a regression test for the {% parent foo %}
// case. parent is supposed to take no arguments (or an empty `()`); silently
// dropping unexpected args would hide real authoring mistakes when the
// formatter rewrites them to `{% parent() %}`.
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

	good := []string{
		`{% parent %}`,
		`{% parent() %}`,
		`{%- parent -%}`,
	}
	for _, src := range good {
		t.Run(src, func(t *testing.T) {
			_, err := NewParser(src)
			assert.NoError(t, err)
		})
	}
}

// TestTrimModifiersRoundTrip is a regression test for whitespace-control
// delimiters. The lexer captures `{%-`, `-%}`, `{{-`, `-}}`, `{#-`, `-#}`
// on the open/close tokens; the AST stores them and the formatter has to
// emit them back verbatim — losing them changes Twig rendered output
// because Twig strips surrounding whitespace at the marked side.
func TestTrimModifiersRoundTrip(t *testing.T) {
	cases := []string{
		`{{- x -}}`,
		`{{ x -}}`,
		`{{- x }}`,
		`{%- if x -%}A{%- else -%}B{%- endif -%}`,
		`{%- block foo -%}body{%- endblock -%}`,
		`{%- for x in xs -%}body{%- endfor -%}`,
		`{%- set x = 1 -%}`,
		`{#- comment -#}`,
		`{%- parent -%}`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			nodes, err := NewParser(src)
			assert.NoError(t, err)
			out := nodes.Dump(0)
			reparsed, err := NewParser(out)
			assert.NoError(t, err)
			assert.Equal(t, out, reparsed.Dump(0), "trim modifiers must round-trip")
			// Spot-check that at least one trim marker survives the formatter.
			if strings.Contains(src, "-%}") {
				assert.True(t, strings.Contains(out, "-%}"), "right-trim on stmt close lost: %s -> %s", src, out)
			}
			if strings.Contains(src, "{%-") {
				assert.True(t, strings.Contains(out, "{%-"), "left-trim on stmt open lost: %s -> %s", src, out)
			}
			if strings.Contains(src, "-}}") {
				assert.True(t, strings.Contains(out, "-}}"), "right-trim on expr close lost: %s -> %s", src, out)
			}
			if strings.Contains(src, "{{-") {
				assert.True(t, strings.Contains(out, "{{-"), "left-trim on expr open lost: %s -> %s", src, out)
			}
		})
	}
}

// TestTwigExprInAttributePreserved is a regression test for the previously
// dropped `<div {{ attributes }}>` pattern. Twig expressions and comments
// embedded in an HTML opening tag must round-trip through Dump verbatim.
func TestTwigExprInAttributePreserved(t *testing.T) {
	cases := []string{
		`<div {{ attributes }}></div>`,
		`<div data-x="y" {{ attributes }} class="z"></div>`,
		`<div {# author note #}></div>`,
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			nodes, err := NewParser(src)
			assert.NoError(t, err)
			out := nodes.Dump(0)
			// The dynamic part must survive Dump.
			if strings.Contains(src, "{{") {
				assert.Contains(t, out, "{{ attributes }}")
			}
			if strings.Contains(src, "{#") {
				assert.Contains(t, out, "{# author note #}")
			}
		})
	}
}

// TestForLoopWithElse is a regression test for the for/else follower
// support: `{% for x in xs %}body{% else %}empty{% endfor %}` is valid
// Twig and was previously rejected with "missing endfor" because the
// generic block parser only accepted stopGenericEndTag.
func TestForLoopWithElse(t *testing.T) {
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

	// Round-trip the formatted output.
	out := nodes.Dump(0)
	assert.True(t, strings.Contains(out, "{% else %}"), "formatter must emit the else branch")
	reparsed, err := NewParser(out)
	assert.NoError(t, err)
	assert.Equal(t, out, reparsed.Dump(0), "for/else must format idempotently")
}
