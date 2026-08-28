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
		// valid absolute http(s) URLs
		"https://developer.shopware.com/docs/products/cli/",
		"https://github.com/shopware/deployment-helper",
		"http://example.com",
		"http://example.com:8080",
		"http://example.com?a=b&c=d",
		"http://example.com#frag",
		"http://example.com/%2F",
		"http://example.com/%2f",
		"http://user:pass@example.com/x",
		"http://[::1]:8080/x",
		"http://127.0.0.1/p",
		"https://a.b-c.d/~user/(x)!,;=",
		// rejected: bad scheme / empty host / delimiters as host
		"ftp://example.com",
		"not-a-url",
		"https://",
		"http://?query",
		"http://#frag",
		// rejected: invalid percent-escapes
		"https://example.com/%zz",
		"http://%zz",
		"http://example.com/%2",
		"http://ex%ample.com",
		// rejected: whitespace / control characters / trailing text
		"http://example.com trailing",
		"http://exa mple.com",
		"http://example.com\n",
		"http://example.com/\x01",
		"http://example.com/\t",
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
