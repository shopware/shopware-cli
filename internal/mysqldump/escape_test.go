package mysqldump

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscape(t *testing.T) {
	input := string([]byte{0, '\n', '\r', '\\', '\'', '"', '\032', 'a'})
	expected := `\0\n\r\\\'\"\Za`
	result := escape(input)
	assert.Equal(t, expected, result)
}

func TestUnescape(t *testing.T) {
	input := `\0\n\r\\\'\"\Za`
	expected := string([]byte{0, '\n', '\r', '\\', '\'', '"', '\032', 'a'})
	result := unescape(input)
	assert.Equal(t, expected, result)
}

func TestEscapeUnescape_RoundTrip(t *testing.T) {
	testCases := []string{
		"simple text",
		"text with\nnewline",
		"text with\rcarriage return",
		`text with "double quotes"`,
		"text with 'single quotes'",
		`text with \backslash`,
		"null\x00byte",
		`json_extract(\'$.taxStatus\')`,
	}

	for _, original := range testCases {
		escaped := escape(original)
		unescaped := unescape(escaped)
		assert.Equal(t, original, unescaped, "round-trip failed for: %q", original)
	}
}

func TestUnescape_UnknownSequencePassthrough(t *testing.T) {
	input := `abc\xyz`
	assert.Equal(t, input, unescape(input))
}

func TestUnescape_TrailingBackslashPassthrough(t *testing.T) {
	input := "abc\\"
	assert.Equal(t, input, unescape(input))
}
