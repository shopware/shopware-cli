package extension

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminWatchOutputURL(t *testing.T) {
	browserURL, err := url.Parse("https://watch.example.test/base/")
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://watch.example.test/base/.shopware-cli/my-plugin/my-plugin-ENTRY.js",
		adminWatchOutputURL(browserURL, "my-plugin", "/my-plugin-ENTRY.js"),
	)
}
