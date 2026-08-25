package extension

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionGetNameAndVersionFromFolderAndZip(t *testing.T) {
	dir := writePluginFixture(t)
	zipPath := buildExtensionZip(t, dir)

	cases := []struct {
		command string
		want    string
	}{
		{"get-name", "FroshTest"},
		{"get-version", "1.0.0"},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			for _, target := range []string{dir, zipPath} {
				out := captureStdout(t, func() {
					require.NoError(t, runExtension(t, tc.command, target))
				})
				assert.Contains(t, out, tc.want)
			}

			err := runExtension(t, tc.command, filepath.Join(t.TempDir(), "missing"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot find path")
		})
	}
}
