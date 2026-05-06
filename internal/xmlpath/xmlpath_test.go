package xmlpath

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentPreservesUnknownNodesAndNamespaces(t *testing.T) {
	doc, err := Parse([]byte(strings.TrimSpace(`<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://example.com/schema.xsd">
    <!-- keep -->
    <meta>
        <name>TestApp</name>
        <version>1.0.0</version>
    </meta>
    <?shopware-cli keep?>
    <future flag="1">
        <nested>value</nested>
    </future>
</manifest>`)))
	require.NoError(t, err)

	doc.Root().Find("meta/version").SetText("2.0.0")

	out, err := doc.MarshalIndent("", "  ")
	require.NoError(t, err)
	output := string(out)

	assert.Contains(t, output, `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`)
	assert.Contains(t, output, `xsi:noNamespaceSchemaLocation="https://example.com/schema.xsd"`)
	assert.Contains(t, output, "<!-- keep -->")
	assert.Contains(t, output, "<?shopware-cli keep?>")
	assert.Contains(t, output, `<future flag="1">`)
	assert.Contains(t, output, "<nested>value</nested>")
	assert.Contains(t, output, "<version>2.0.0</version>")
	assert.NotContains(t, output, "_xmlns")
}

func TestFindAllSetAttrAndRemoveAll(t *testing.T) {
	doc, err := Parse([]byte(`<manifest>
    <webhooks>
        <webhook name="a" url="https://old.example/a"/>
        <webhook name="b" url="https://old.example/b"/>
    </webhooks>
    <setup>
        <secret>secret</secret>
    </setup>
</manifest>`))
	require.NoError(t, err)

	for _, webhook := range doc.Root().FindAll("webhooks/webhook") {
		value, ok := webhook.Attr("url")
		require.True(t, ok)
		webhook.SetAttr("url", strings.Replace(value, "old.example", "new.example", 1))
	}
	assert.Equal(t, 1, doc.Root().RemoveAll("setup/secret"))

	out, err := doc.MarshalIndent("", "  ")
	require.NoError(t, err)
	output := string(out)

	assert.Contains(t, output, `url="https://new.example/a"`)
	assert.Contains(t, output, `url="https://new.example/b"`)
	assert.NotContains(t, output, "<secret>")
}

func TestAppendChildInOrder(t *testing.T) {
	doc, err := Parse([]byte(`<manifest>
    <meta>
        <name>TestApp</name>
        <license>MIT</license>
    </meta>
</manifest>`))
	require.NoError(t, err)

	meta := doc.Root().Find("meta")
	description := meta.AppendChildInOrder("description", []string{"name", "label", "description", "author", "license"})
	description.SetText("Description")
	label := meta.AppendChildInOrder("label", []string{"name", "label", "description", "author", "license"})
	label.SetText("Label")

	out, err := doc.MarshalIndent("", "  ")
	require.NoError(t, err)
	output := string(out)

	assertElementBefore(t, output, "<name>", "<label>")
	assertElementBefore(t, output, "<label>", "<description>")
	assertElementBefore(t, output, "<description>", "<license>")
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"malformed token", `<manifest`},
		{"multiple roots", `<first></first><second></second>`},
		{"nested parse error", `<manifest><meta>`},
		{"missing root", `   `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.input))
			require.Error(t, err)
		})
	}
}

func TestPathHelpersAndText(t *testing.T) {
	doc, err := Parse([]byte(`<manifest>
    <meta>prefix<name>TestApp</name>suffix</meta>
</manifest>`))
	require.NoError(t, err)

	root := doc.Root()

	_, ok := root.Attr("missing")
	assert.False(t, ok)
	root.SetAttr("generated", "true")
	generated, ok := root.Attr("generated")
	require.True(t, ok)
	assert.Equal(t, "true", generated)

	assert.Nil(t, root.Find("setup"))
	assert.Same(t, root, root.FindAll("")[0])
	assert.Same(t, root, root.FindAll("manifest")[0])
	assert.Same(t, root, root.EnsurePath(""))

	version := root.EnsurePath("manifest/meta/version")
	version.SetText("1.0.0")
	assert.Same(t, version, root.EnsurePath("meta/version"))
	assert.Equal(t, "prefixsuffix", root.Find("meta").Text())

	tail := root.AppendChild("tail")
	tail.SetText("value")
	assert.Equal(t, 0, root.RemoveAll(""))
	assert.Equal(t, 1, root.RemoveAll("manifest/tail"))
	assert.Equal(t, 0, root.removeAll(nil))

	out, err := doc.MarshalIndent("", "  ")
	require.NoError(t, err)
	output := string(out)

	assert.Contains(t, output, `generated="true"`)
	assert.Contains(t, output, "<version>1.0.0</version>")
	assert.NotContains(t, output, "<tail>")
}

func TestAppendChildInOrderEdgeCases(t *testing.T) {
	t.Run("unknown target appends", func(t *testing.T) {
		doc, err := Parse([]byte(`<manifest><meta><name>TestApp</name></meta></manifest>`))
		require.NoError(t, err)

		meta := doc.Root().Find("meta")
		custom := meta.AppendChildInOrder("custom", []string{"name", "label"})
		custom.SetText("value")

		out, err := doc.MarshalIndent("", "  ")
		require.NoError(t, err)
		assertElementBefore(t, string(out), "<name>", "<custom>")
	})

	t.Run("known target inserts before unknown after lower ranked element", func(t *testing.T) {
		doc, err := Parse([]byte(`<manifest><meta><name>TestApp</name><future>keep</future></meta></manifest>`))
		require.NoError(t, err)

		meta := doc.Root().Find("meta")
		label := meta.AppendChildInOrder("label", []string{"name", "label", "description"})
		label.SetText("Label")

		out, err := doc.MarshalIndent("", "  ")
		require.NoError(t, err)
		output := string(out)

		assertElementBefore(t, output, "<name>", "<label>")
		assertElementBefore(t, output, "<label>", "<future>")
	})

	t.Run("known target appends after lower ranked element", func(t *testing.T) {
		doc, err := Parse([]byte(`<manifest><meta><name>TestApp</name></meta></manifest>`))
		require.NoError(t, err)

		meta := doc.Root().Find("meta")
		label := meta.AppendChildInOrder("label", []string{"name", "label", "description"})
		label.SetText("Label")

		out, err := doc.MarshalIndent("", "  ")
		require.NoError(t, err)
		assertElementBefore(t, string(out), "<name>", "<label>")
	})
}

func TestNamespaceHelpers(t *testing.T) {
	prefixed, err := Parse([]byte(`<x:manifest xmlns:x="urn:test"><x:meta x:flag="1"/></x:manifest>`))
	require.NoError(t, err)

	out, err := prefixed.MarshalIndent("", "")
	require.NoError(t, err)
	output := string(out)
	assert.Contains(t, output, `<x:manifest`)
	assert.Contains(t, output, `<x:meta`)
	assert.Contains(t, output, `x:flag="1"`)

	defaultNamespace, err := Parse([]byte(`<manifest xmlns="urn:test"><meta/></manifest>`))
	require.NoError(t, err)

	out, err = defaultNamespace.MarshalIndent("", "")
	require.NoError(t, err)
	output = string(out)
	assert.Contains(t, output, `<manifest xmlns="urn:test">`)
	assert.Contains(t, output, `<meta></meta>`)

	assert.Equal(t, "plain", localName("plain"))
	assert.Equal(t, "name", localName("x:name"))
	assert.Equal(t, xml.Name{Space: "urn:missing", Local: "flag"}, attrNameForMarshal(xml.Name{Space: "urn:missing", Local: "flag"}, nil))
}

func TestMarshalErrorPaths(t *testing.T) {
	_, err := (&Document{nodes: []node{tokenNode(xml.EndElement{Name: xml.Name{Local: "orphan"}})}}).MarshalIndent("", "")
	require.Error(t, err)

	_, err = (&Document{nodes: []node{tokenNode(xml.StartElement{Name: xml.Name{Local: "open"}})}}).MarshalIndent("", "")
	require.Error(t, err)

	encoder := xml.NewEncoder(&bytes.Buffer{})
	err = encodeElement(encoder, &Element{start: xml.StartElement{Name: xml.Name{}}})
	require.Error(t, err)

	encoder = xml.NewEncoder(&bytes.Buffer{})
	err = encodeElement(encoder, &Element{
		start: xml.StartElement{Name: xml.Name{Local: "root"}},
		children: []node{
			tokenNode(xml.ProcInst{Target: "xml"}),
		},
	})
	require.Error(t, err)
}

func TestCloneTokenCases(t *testing.T) {
	cloned, ok := cloneToken(xml.Directive("DOCTYPE manifest"))
	require.True(t, ok)
	assert.Equal(t, xml.Directive("DOCTYPE manifest"), cloned)

	cloned, ok = cloneToken(xml.StartElement{Name: xml.Name{Local: "manifest"}})
	require.True(t, ok)
	assert.Equal(t, xml.StartElement{Name: xml.Name{Local: "manifest"}}, cloned)

	cloned, ok = cloneToken(xml.ProcInst{Target: "xml", Inst: []byte(`version="1.0"`)})
	assert.False(t, ok)
	assert.Nil(t, cloned)
}

func assertElementBefore(t *testing.T, output, first, second string) {
	t.Helper()

	firstIndex := strings.Index(output, first)
	secondIndex := strings.Index(output, second)

	require.NotEqual(t, -1, firstIndex, "%s not found in output", first)
	require.NotEqual(t, -1, secondIndex, "%s not found in output", second)
	assert.Less(t, firstIndex, secondIndex)
}
