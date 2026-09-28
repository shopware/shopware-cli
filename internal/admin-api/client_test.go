package admin_sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/cliversion"
)

func TestNewRawRequestSetsUserAgent(t *testing.T) {
	client := &Client{url: "https://example.com"}
	request, err := client.NewRawRequest(NewApiContext(t.Context()), http.MethodGet, "/api/test", nil)

	require.NoError(t, err)
	assert.Equal(t, cliversion.UserAgent(), request.Header.Get("User-Agent"))
}
