package dev

import (
	"testing"

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
