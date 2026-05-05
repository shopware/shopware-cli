package xmlpath

import (
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

func assertElementBefore(t *testing.T, output, first, second string) {
	t.Helper()

	firstIndex := strings.Index(output, first)
	secondIndex := strings.Index(output, second)

	require.NotEqual(t, -1, firstIndex, "%s not found in output", first)
	require.NotEqual(t, -1, secondIndex, "%s not found in output", second)
	assert.Less(t, firstIndex, secondIndex)
}
