package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormattingOfHTML(t *testing.T) {
	t.Parallel()
	swBlock := &ElementNode{
		Tag: "sw-button",
		Attributes: NodeList{
			&Attribute{
				Key:   "label",
				Value: "Click me",
			},
			&Attribute{
				Key:   "variant",
				Value: "primary",
			},
		},
	}

	node := &ElementNode{Tag: "template", Attributes: NodeList{}, Children: NodeList{swBlock}}

	assert.Equal(t, `<template>
    <sw-button
        label="Click me"
        variant="primary"
    ></sw-button>
</template>`, node.Dump(0))

	simpleButton := &ElementNode{
		Tag: "sw-button",
		Children: NodeList{
			&RawNode{Text: "Click me"},
		},
	}

	assert.Equal(t, `<sw-button>Click me</sw-button>`, simpleButton.Dump(0))
}

func TestFormatting(t *testing.T) {
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		t.Run(f.Name(), func(t *testing.T) {
			name := f.Name()

			data, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}

			stringData := string(data)
			stringParts := strings.SplitN(stringData, "-----", 3)

			if len(stringParts) < 2 {
				t.Fatalf("file %s does not contain expected delimiter", name)
			}

			stringParts[0] = strings.Trim(stringParts[0], "\n")
			stringParts[1] = strings.Trim(stringParts[1], "\n")

			parsed, err := NewAdminParser(stringParts[0])
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, stringParts[1], parsed.Dump(0))

			parsed, err = NewAdminParser(parsed.Dump(0))
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, stringParts[1], parsed.Dump(0))

			if len(stringParts) < 3 {
				return
			}

			stringParts[2] = strings.Trim(stringParts[2], "\n")

			parsed, err = NewStorefrontParser(stringParts[0])
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, stringParts[2], parsed.Dump(0))

			parsed, err = NewStorefrontParser(parsed.Dump(0))
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, stringParts[2], parsed.Dump(0))
		})
	}
}

func TestChangeElement(t *testing.T) {
	t.Parallel()
	node, err := NewParser(`<sw-select @update:value="onUpdateValue"/>`)
	assert.NoError(t, err)
	TraverseNode(node, func(n *ElementNode) {
		n.Tag = "mt-select"
		var newAttributes NodeList
		for _, attr := range n.Attributes {
			if attribute, ok := attr.(Attribute); ok {
				if attribute.Key == "@update:value" {
					attribute.Key = "@update:modelValue"
				}
				newAttributes = append(newAttributes, attribute)
			} else {
				newAttributes = append(newAttributes, attr)
			}
		}
		n.Attributes = newAttributes
	})
	assert.Equal(t, `<mt-select @update:modelValue="onUpdateValue"/>`, node.Dump(0))
}

func TestBlockParsing(t *testing.T) {
	t.Parallel()
	input := `{% block name %}{% endblock %}`

	node, err := NewParser(input)
	assert.NoError(t, err)

	assert.Equal(t, input, node.Dump(0))

	block, ok := node[0].(*TwigBlockNode)
	assert.True(t, ok)
	assert.Equal(t, "name", block.Name)
}
