package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuote(t *testing.T) {
	assert.Equal(t, "'/var/www'", Quote("/var/www"))
	assert.Equal(t, `'it'\''s'`, Quote("it's"))
}
