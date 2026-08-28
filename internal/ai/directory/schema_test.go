package directory

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaURLPatternMatchesValidator locks the published schema's URL pattern
// to the runtime validator: for each case, the pattern and isAbsoluteHTTPURL
// must agree, so the schema never accepts a URL the CLI rejects (or vice versa).
func TestSchemaURLPatternMatchesValidator(t *testing.T) {
	re := regexp.MustCompile(httpsURLPattern)

	for _, s := range []string{
		"https://developer.shopware.com/docs/products/cli/",
		"https://github.com/shopware/deployment-helper",
		"http://example.com",
		"http://example.com?query",
		"http://example.com#frag",
		"http://?query",
		"http://#frag",
		"http://example.com trailing",
		"ftp://example.com",
		"not-a-url",
		"https://",
	} {
		assert.Equalf(t, isAbsoluteHTTPURL(s), re.MatchString(s),
			"schema pattern and Validate disagree for %q", s)
	}
}

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
