package phplint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDownloadPHPFile(t *testing.T) {
	if !IsPHPWasmCached("7.4") {
		t.Skip("PHP WASM binary not cached; run once with network to download")
	}

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	_, err := findPHPWasmFile(t.Context(), "7.4")
	assert.NoError(t, err)
}
