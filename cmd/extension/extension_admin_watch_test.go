package extension

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeAdminWatchStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{Addr: "127.0.0.1:0"}

	done := make(chan error, 1)
	go func() {
		done <- serveAdminWatch(ctx, server)
	}()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("admin watch server did not stop after context cancellation")
	}
}

func TestAdminWatchOutputURL(t *testing.T) {
	browserURL, err := url.Parse("https://watch.example.test/base/")
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://watch.example.test/base/.shopware-cli/my-plugin/my-plugin-ENTRY.js",
		adminWatchOutputURL(browserURL, "my-plugin", "/my-plugin-ENTRY.js"),
	)
}
