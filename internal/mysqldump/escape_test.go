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
