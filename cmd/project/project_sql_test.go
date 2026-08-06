package project

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/sqlshell"
)

func newSQLFormatCommand(t *testing.T, formatFlag string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "")
	cmd.SetOut(&bytes.Buffer{})

	if formatFlag != "" {
		require.NoError(t, cmd.Flags().Set("format", formatFlag))
	}

	return cmd
}

func TestResolveSQLFormatDefaultsToTSVWithoutTerminal(t *testing.T) {
	format, err := resolveSQLFormat(newSQLFormatCommand(t, ""))
	require.NoError(t, err)

	assert.Equal(t, sqlshell.FormatTSV, format)
}

func TestResolveSQLFormatExplicit(t *testing.T) {
	format, err := resolveSQLFormat(newSQLFormatCommand(t, "json"))
	require.NoError(t, err)

	assert.Equal(t, sqlshell.FormatJSON, format)
}

func TestResolveSQLFormatInvalid(t *testing.T) {
	_, err := resolveSQLFormat(newSQLFormatCommand(t, "xml"))
	assert.ErrorContains(t, err, "unknown format")
}

func TestIsTerminalStream(t *testing.T) {
	assert.False(t, isTerminalStream(strings.NewReader("not a file")))

	path := filepath.Join(t.TempDir(), "plain.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	assert.False(t, isTerminalStream(file), "a regular file is not a terminal")
}
