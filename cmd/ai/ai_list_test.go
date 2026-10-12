package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

func TestWriteListJSON(t *testing.T) {
	entries := []directory.Integration{{Name: "x", Description: "d", Repository: "https://example.test/repo"}}

	var buf bytes.Buffer
	require.NoError(t, writeListJSON(&buf, entries))

	assert.JSONEq(t, `[{"name":"x","description":"d","repository":"https://example.test/repo"}]`, buf.String())
}

func TestWriteListJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeListJSON(&buf, nil))

	assert.JSONEq(t, `[]`, buf.String())
}
