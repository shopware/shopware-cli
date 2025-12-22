package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangelogParsing(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n\n- Test\n- Test2\n# 2.0.0\n- Test3\n- Test4\n")
	assert.NoError(t, err)

	assert.Equal(t, "<ul>\n<li>Test</li>\n<li>Test2</li>\n</ul>\n", content["1.0.0"])
	assert.Equal(t, "<ul>\n<li>Test3</li>\n<li>Test4</li>\n</ul>\n", content["2.0.0"])
}

func TestChangelogParsingWhitespaces(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n \n- Test\n- Test2\n# 2.0.0\n- Test3\n - Test4\n")
	assert.NoError(t, err)

	assert.Equal(t, "<ul>\n<li>Test</li>\n<li>Test2</li>\n</ul>\n", content["1.0.0"])
	assert.Equal(t, "<ul>\n<li>Test3</li>\n<li>Test4</li>\n</ul>\n", content["2.0.0"])
}
