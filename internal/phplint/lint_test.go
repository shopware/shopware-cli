package phplint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLintTestData(t *testing.T) {
	if !IsPHPWasmCached("8.2") {
		t.Skip("PHP WASM binary not cached; run once with network to download")
	}

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	supportedPHPVersions := []string{"7.3", "7.4", "8.1", "8.2", "8.3"}

	for _, version := range supportedPHPVersions {
		errors, err := LintFolder(t.Context(), version, "testdata")

		assert.NoError(t, err)

		assert.Len(t, errors, 1)

		assert.Equal(t, "invalid.php", errors[0].File)

		if version == "7.3" {
			assert.Contains(t, errors[0].Message, "Errors parsing invalid.php")
		} else {
			assert.Contains(t, errors[0].Message, "syntax error, unexpected end of file")
		}
	}
}
