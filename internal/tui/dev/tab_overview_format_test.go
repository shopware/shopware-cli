package dev

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.5 KB", formatBytes(1536))
	assert.Equal(t, "1.0 MB", formatBytes(1<<20))
	assert.Equal(t, "2.0 GB", formatBytes(2<<30))
	assert.Equal(t, "1.5 GB", formatBytes(int64(1.5*(1<<30))))
}

func TestWatchLinkLabel(t *testing.T) {
	t.Parallel()

	// Proxy hostnames collapse to their leading label.
	assert.Equal(t, "storefront-watch", watchLinkLabel("https://storefront-watch.shop9.shopware.local"))
	assert.Equal(t, "admin-watch", watchLinkLabel("https://admin-watch.shop9.shopware.local"))
	// Plain local URLs keep host:port, since the port is the distinguishing part.
	assert.Equal(t, "127.0.0.1:9998", watchLinkLabel("http://127.0.0.1:9998"))
	assert.Equal(t, "localhost:5173", watchLinkLabel("http://localhost:5173"))
	// Garbage falls back to the raw string.
	assert.Equal(t, "", watchLinkLabel(""))
}

func TestFormatUptime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "—", formatUptime(0))
	assert.Equal(t, "45m", formatUptime(45*time.Minute))
	assert.Equal(t, "1h 24m", formatUptime(time.Hour+24*time.Minute))
	assert.Equal(t, "2h 0m", formatUptime(2*time.Hour))
	assert.Equal(t, "3d 2h", formatUptime(3*24*time.Hour+2*time.Hour+30*time.Minute))
}
