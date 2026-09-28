package admin_sdk

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUserAgent(t *testing.T) {
	previous := httpUserAgent
	t.Cleanup(func() { httpUserAgent = previous })

	SetUserAgent("shopware-cli/test")

	assert.Equal(t, "shopware-cli/test", httpUserAgent)
}

func TestNewRawRequestSetsUserAgent(t *testing.T) {
	previous := httpUserAgent
	t.Cleanup(func() { httpUserAgent = previous })
	SetUserAgent("shopware-cli/test")

	client := &Client{url: "https://example.com"}
	request, err := client.NewRawRequest(NewApiContext(t.Context()), http.MethodGet, "/api/test", nil)

	require.NoError(t, err)
	assert.Equal(t, "shopware-cli/test", request.Header.Get("User-Agent"))
}
