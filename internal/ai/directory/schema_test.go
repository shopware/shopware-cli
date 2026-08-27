package directory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaJSONInSync fails when the committed schema.json drifts from the Go
// types. Regenerate with `go run ./scripts` after changing the manifest types.
func TestSchemaJSONInSync(t *testing.T) {
	got, err := GenerateSchemaJSON()
	require.NoError(t, err)

	want, err := os.ReadFile("schema.json")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got),
		"internal/ai/directory/schema.json is out of sync with the Go types; regenerate it with `go run ./scripts`")
}
