package system

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountingReader(t *testing.T) {
	counting := &CountingReader{Reader: strings.NewReader("hello world")}

	content, err := io.ReadAll(counting)
	require.NoError(t, err)

	assert.Equal(t, "hello world", string(content))
	assert.Equal(t, int64(11), counting.BytesRead())
}
