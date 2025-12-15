package twigparser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockParsing(t *testing.T) {
	template := `{% block content %}
{% block page_account_address_form_create_personal %}
            {{ parent() }}
        {% endblock %}

        {% block page_account_address_form_create_general %}
            {{ parent() }}
        {% endblock %}
{% endblock %}`

	nodes, err := ParseTemplate(template)

	assert.NoError(t, err)

	assert.NotNil(t, nodes.FindBlock("content"))
	assert.NotNil(t, nodes.FindBlock("page_account_address_form_create_personal"))
	assert.NotNil(t, nodes.FindBlock("page_account_address_form_create_general"))
}

func TestTraversing(t *testing.T) {
	template := `{% block content %}
{{ parent() }}
{% endblock %}`

	nodes, err := ParseTemplate(template)

	assert.NoError(t, err)

	nodes.Traverse(func(node Node) Node {
		if block, ok := node.(*BlockNode); ok {
			filteredNodes := block.Children.RemoveWhitespace()

			if _, ok := filteredNodes[0].(*ParentNode); ok && len(filteredNodes) == 1 {
				return &TextNode{Text: fmt.Sprintf("{{ block(\"%s\") }}", block.Name)}
			}
		}

		return node
	})

	assert.Equal(t, "{{ block(\"content\") }}", nodes.Dump())
}

func TestSwExtendsParsing(t *testing.T) {
	testcases := []struct {
		template string
		path     string
		scopes   []string
	}{
		{
			template: `{% sw_extends 'foo.twig' %}`,
			path:     "foo.twig",
			scopes:   []string{},
		},
		{
			template: `{% sw_extends {
    template: '@Storefront/storefront/page/checkout/finish/finish-details.html.twig',
    scopes: ['default', 'subscription']
    }
%}`,
			path:   "@Storefront/storefront/page/checkout/finish/finish-details.html.twig",
			scopes: []string{"default", "subscription"},
		},
	}

	for _, tc := range testcases {
		nodes, err := ParseTemplate(tc.template)

		assert.NoError(t, err)

		extends := nodes.Extends()

		assert.NotNil(t, extends)
		assert.Equal(t, tc.path, extends.Template)
		assert.Equal(t, tc.scopes, extends.Scopes)
	}
}

func TestPrintNodeParsing(t *testing.T) {
	template := `{{ a_variable }}`
	nodes, err := ParseTemplate(template)
	assert.NoError(t, err)

	var printNode *PrintNode
	for _, node := range nodes {
		if pn, ok := node.(*PrintNode); ok {
			printNode = pn
			break
		}
	}
	assert.NotNil(t, printNode)
	assert.Equal(t, "a_variable", printNode.Expression)
	assert.Equal(t, "{{ a_variable }}", nodes.Dump())
}

func TestDeprecatedNodeParsing(t *testing.T) {
	template := `{% deprecated 'The "base.html.twig" template is deprecated, use "layout.html.twig" instead.' %}`
	nodes, err := ParseTemplate(template)
	assert.NoError(t, err)

	var deprecatedNode *DeprecatedNode
	for _, node := range nodes {
		if dn, ok := node.(*DeprecatedNode); ok {
			deprecatedNode = dn
			break
		}
	}
	assert.NotNil(t, deprecatedNode)
	expectedMsg := `The "base.html.twig" template is deprecated, use "layout.html.twig" instead.`
	assert.Equal(t, expectedMsg, deprecatedNode.Message)
	assert.Equal(t, "{% deprecated '"+expectedMsg+"' %}", nodes.Dump())
}

func TestSetNodeParsing(t *testing.T) {
	// Inline set assignment.
	inlineTemplate := `{% set name = 'Fabien' %}`
	nodes, err := ParseTemplate(inlineTemplate)
	assert.NoError(t, err)

	var inlineSet *SetNode
	for _, node := range nodes {
		if sn, ok := node.(*SetNode); ok && sn.IsBlock == false {
			inlineSet = sn
			break
		}
	}
	assert.NotNil(t, inlineSet)
	assert.Equal(t, []string{"name"}, inlineSet.Variables)
	assert.Equal(t, []string{"'Fabien'"}, inlineSet.Values)

	// Block set assignment.
	blockTemplate := `{% set content %}
    <div id="pagination">
        Pagination here.
    </div>
{% endset %}`
	nodes, err = ParseTemplate(blockTemplate)
	assert.NoError(t, err)

	var blockSet *SetNode
	for _, node := range nodes {
		if sn, ok := node.(*SetNode); ok && sn.IsBlock == true {
			blockSet = sn
			break
		}
	}
	assert.NotNil(t, blockSet)
	assert.Equal(t, []string{"content"}, blockSet.Variables)
	// Check that the dumped content contains the inner HTML.
	dumped := blockSet.Dump()
	assert.Contains(t, dumped, "<div id=\"pagination\">")
}

func TestAutoescapeNodeParsing(t *testing.T) {
	template := `{% autoescape %}
    Everything will be automatically escaped.
    {% endautoescape %}`
	nodes, err := ParseTemplate(template)
	assert.NoError(t, err)

	var autoNode *AutoescapeNode
	for _, node := range nodes {
		if an, ok := node.(*AutoescapeNode); ok {
			autoNode = an
			break
		}
	}
	assert.NotNil(t, autoNode)
	// Default strategy "html" is used.
	assert.Equal(t, "html", autoNode.Strategy)

	dumped := autoNode.Dump()
	assert.Contains(t, dumped, "{% autoescape %}")
	assert.Contains(t, dumped, "{% endautoescape %}")
}
