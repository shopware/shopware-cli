package phplint

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDownloadPHPFile(t *testing.T) {
	if os.Getenv("NIX_CC") != "" || os.Getenv("SHOPWARE_CLI_NO_NETWORK") != "" {
		t.Skip("Downloading does not work without network access")
	}

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	_, err := findPHPWasmFile(t.Context(), "7.4")
	assert.NoError(t, err)
}
