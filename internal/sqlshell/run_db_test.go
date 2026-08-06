package sqlshell

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunQueryAndExec(t *testing.T) {
	fakeQueries.Store("SELECT id, name FROM tax", fakeTable{
		cols:  []string{"id", "name"},
		types: []string{"BIGINT", "VARCHAR"},
		rows: [][]driver.Value{
			{[]byte("1"), []byte("Standard")},
			{[]byte("2"), nil},
		},
	})

	var out strings.Builder

	err := Run(t.Context(), openFakeDB(t), "SELECT id, name FROM tax; UPDATE tax SET name = 'x';", &out, FormatTSV)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "id\tname\n1\tStandard\n2\tNULL\n")
	assert.Contains(t, out.String(), "Query OK, 3 rows affected")
}

func TestRunTrailingStatementWithoutSemicolon(t *testing.T) {
	var out strings.Builder

	err := Run(t.Context(), openFakeDB(t), "UPDATE t SET one_row = 1", &out, FormatTSV)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "Query OK, 1 row affected")
}

func TestRunTableFormat(t *testing.T) {
	fakeQueries.Store("SELECT v FROM t", fakeTable{
		cols:  []string{"v"},
		types: []string{"VARCHAR"},
		rows:  [][]driver.Value{{[]byte("x")}},
	})

	var out strings.Builder

	require.NoError(t, Run(t.Context(), openFakeDB(t), "SELECT v FROM t", &out, FormatTable))

	assert.Contains(t, out.String(), "| v |")
	assert.Contains(t, out.String(), "| x |")
	assert.Contains(t, out.String(), "1 row in set")
}

func TestRunStatementBinaryHexAndNumericJSON(t *testing.T) {
	fakeQueries.Store("SELECT id, price, name FROM product", fakeTable{
		cols:  []string{"id", "price", "name"},
		types: []string{"BINARY", "DECIMAL", "VARCHAR"},
		rows:  [][]driver.Value{{[]byte{0xab, 0xcd}, []byte("19.99"), []byte("Fancy")}},
	})

	var out strings.Builder

	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "SELECT id, price, name FROM product", &out, FormatJSON))

	assert.JSONEq(t, `[{"id":"0xABCD","price":19.99,"name":"Fancy"}]`, out.String())
}

func TestRunStatementExecJSON(t *testing.T) {
	var out strings.Builder

	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "DELETE FROM t", &out, FormatJSON))

	assert.JSONEq(t, `{"rows_affected":3}`, out.String())
}

func TestRunStatementReturningUsesQueryPath(t *testing.T) {
	fakeQueries.Store("INSERT INTO t (v) VALUES ('a') RETURNING id", fakeTable{
		cols:  []string{"id"},
		types: []string{"BIGINT"},
		rows:  [][]driver.Value{{[]byte("7")}},
	})

	var out strings.Builder

	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "INSERT INTO t (v) VALUES ('a') RETURNING id", &out, FormatTSV))

	assert.Equal(t, "id\n7\n", out.String())
}

func TestRunStatementSkipsCommentsAndDelimiter(t *testing.T) {
	var out strings.Builder

	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "-- nothing here", &out, FormatTSV))
	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "DELIMITER ;", &out, FormatTSV))

	assert.Empty(t, out.String())
}

func TestRunStatementEmptyResultSetColumns(t *testing.T) {
	fakeQueries.Store("CALL cleanup", fakeTable{})

	var out strings.Builder

	require.NoError(t, RunStatement(t.Context(), openFakeDB(t), "CALL cleanup", &out, FormatTSV))
	assert.Empty(t, out.String())
}

func TestRunMultiStatementErrorIncludesSummary(t *testing.T) {
	fakeQueries.Store("SELECT 1", fakeTable{
		cols:  []string{"1"},
		types: []string{"BIGINT"},
		rows:  [][]driver.Value{{[]byte("1")}},
	})

	var out strings.Builder

	err := Run(t.Context(), openFakeDB(t), "SELECT 1; SELECT boom; SELECT 1;", &out, FormatTSV)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "SELECT boom")
	assert.Contains(t, err.Error(), "query exploded")
}

func TestRunSingleStatementErrorHasNoSummary(t *testing.T) {
	var out strings.Builder

	err := Run(t.Context(), openFakeDB(t), "SELECT boom", &out, FormatTSV)
	require.Error(t, err)

	assert.Equal(t, "query exploded", err.Error())
}

func TestShellExecutesAndRecoversFromErrors(t *testing.T) {
	fakeQueries.Store("SELECT 1", fakeTable{
		cols:  []string{"1"},
		types: []string{"BIGINT"},
		rows:  [][]driver.Value{{[]byte("1")}},
	})
	fakeQueries.Store("SELECT\n2", fakeTable{
		cols:  []string{"2"},
		types: []string{"BIGINT"},
		rows:  [][]driver.Value{{[]byte("2")}},
	})

	input := "SELECT 1;\n" + // simple statement
		"SELECT boom;\n" + // error must not end the session
		"SELECT\n2;\n" + // statement spanning two lines
		"UPDATE t SET one_row = 1" // no semicolon: runs on EOF

	var out, errOut strings.Builder

	err := Shell(t.Context(), openFakeDB(t), strings.NewReader(input), &out, &errOut, FormatTSV)
	require.NoError(t, err)

	assert.Contains(t, out.String(), "1\n1\n")
	assert.Contains(t, out.String(), "2\n2\n")
	assert.Contains(t, out.String(), "Query OK, 1 row affected")
	assert.Contains(t, out.String(), promptContinuation)
	assert.Contains(t, errOut.String(), "query exploded")
}

func TestShellStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var out, errOut strings.Builder

	err := Shell(ctx, openFakeDB(t), strings.NewReader("SELECT 1;\n"), &out, &errOut, FormatTSV)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShellExitCommandStopsSession(t *testing.T) {
	var out, errOut strings.Builder

	err := Shell(t.Context(), openFakeDB(t), strings.NewReader("exit\nSELECT boom;\n"), &out, &errOut, FormatTSV)
	require.NoError(t, err)

	assert.Empty(t, errOut.String(), "statements after exit must not run")
}
