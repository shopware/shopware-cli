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
	"github.com/shopware/shopware-cli/internal/system"
)

func newSQLFormatCommand(t *testing.T, formatFlag string) *cobra.Command {
	t.Helper()

	return newSQLCommand(t, formatFlag, "")
}

func newSQLCommand(t *testing.T, formatFlag, fileFlag string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "", "")
	cmd.Flags().String("file", "", "")
	cmd.SetOut(&bytes.Buffer{})

	if formatFlag != "" {
		require.NoError(t, cmd.Flags().Set("format", formatFlag))
	}

	if fileFlag != "" {
		require.NoError(t, cmd.Flags().Set("file", fileFlag))
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

func TestResolveSQLInputFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sql")
	require.NoError(t, os.WriteFile(path, []byte("SELECT 1;\nUPDATE tax SET tax_rate = 19;"), 0o644))

	script, provided, err := resolveSQLInput(newSQLCommand(t, "", path), nil)
	require.NoError(t, err)

	assert.True(t, provided)
	assert.Equal(t, "SELECT 1;\nUPDATE tax SET tax_rate = 19;", script)
}

func TestResolveSQLInputFileIgnoresStdin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sql")
	require.NoError(t, os.WriteFile(path, []byte("SELECT id FROM tax"), 0o644))

	cmd := newSQLCommand(t, "", path)
	cmd.SetIn(strings.NewReader("SELECT * FROM product"))

	script, provided, err := resolveSQLInput(cmd, nil)
	require.NoError(t, err)

	assert.True(t, provided)
	assert.Equal(t, "SELECT id FROM tax", script)
}

func TestResolveSQLInputFileAndQueryConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script.sql")
	require.NoError(t, os.WriteFile(path, []byte("SELECT 1"), 0o644))

	_, _, err := resolveSQLInput(newSQLCommand(t, "", path), []string{"SELECT 2"})
	assert.ErrorContains(t, err, "cannot pass a query together with --file")
}

func TestResolveSQLInputMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sql")

	_, _, err := resolveSQLInput(newSQLCommand(t, "", path), nil)
	assert.ErrorContains(t, err, "failed to read SQL file")
	assert.ErrorContains(t, err, path)
}

func TestResolveSQLInputFromArgs(t *testing.T) {
	script, provided, err := resolveSQLInput(newSQLCommand(t, "", ""), []string{"SELECT", "id", "FROM", "tax"})
	require.NoError(t, err)

	assert.True(t, provided)
	assert.Equal(t, "SELECT id FROM tax", script)
}

func TestResolveSQLInputFromStdin(t *testing.T) {
	cmd := newSQLCommand(t, "", "")
	cmd.SetIn(strings.NewReader("INSERT INTO tax (tax_rate) VALUES (7)"))

	script, provided, err := resolveSQLInput(cmd, nil)
	require.NoError(t, err)

	assert.True(t, provided)
	assert.Equal(t, "INSERT INTO tax (tax_rate) VALUES (7)", script)
}

func TestResolveSQLInputEmptyStdinIsMissing(t *testing.T) {
	for _, stdin := range []string{"", "  \n\t"} {
		cmd := newSQLCommand(t, "", "")
		cmd.SetIn(strings.NewReader(stdin))

		script, provided, err := resolveSQLInput(cmd, nil)
		require.NoError(t, err, "stdin %q", stdin)
		assert.False(t, provided, "stdin %q", stdin)
		assert.Equal(t, stdin, script)

		err = errIfNoSQLInput(system.WithInteraction(t.Context(), false), provided, isTerminalStream(cmd.InOrStdin()))
		assert.ErrorContains(t, err, "no query given and interaction is disabled", "stdin %q", stdin)
	}
}

func TestProjectSQLCmdRegistersFileFlag(t *testing.T) {
	flag := projectSQLCmd.Flags().Lookup("file")
	require.NotNil(t, flag)
	assert.Equal(t, "path to a SQL file to execute (instead of a query argument or stdin)", flag.Usage)
}

func TestErrIfNoSQLInput(t *testing.T) {
	assert.NoError(t, errIfNoSQLInput(t.Context(), true, false))
	assert.NoError(t, errIfNoSQLInput(t.Context(), false, true))
	assert.NoError(t, errIfNoSQLInput(system.WithInteraction(t.Context(), false), true, true))

	err := errIfNoSQLInput(system.WithInteraction(t.Context(), false), false, true)
	assert.ErrorContains(t, err, "no query given and interaction is disabled")

	err = errIfNoSQLInput(t.Context(), false, false)
	assert.ErrorContains(t, err, "no query given and interaction is disabled")
}
