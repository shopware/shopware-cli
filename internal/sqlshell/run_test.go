package sqlshell

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormat(t *testing.T) {
	for _, valid := range []string{"table", "tsv", "json"} {
		format, err := ParseFormat(valid)
		assert.NoError(t, err)
		assert.Equal(t, Format(valid), format)
	}

	_, err := ParseFormat("xml")
	assert.Error(t, err)
}

func TestFirstKeyword(t *testing.T) {
	cases := map[string]string{
		"SELECT 1":                        "select",
		"  select * from t":               "select",
		"(SELECT 1) UNION (SELECT 2)":     "select",
		"/* leading */ SHOW TABLES":       "show",
		"-- comment\nUPDATE t SET a = 1":  "update",
		"# comment\nDELETE FROM t":        "delete",
		"WITH x AS (SELECT 1) SELECT 1":   "with",
		"INSERT INTO t VALUES (1)":        "insert",
		"EXPLAIN(FORMAT=JSON) SELECT 1":   "explain",
		"/* only a comment, no keyword*/": "",
		"-- only a comment":               "",

		// MySQL executable comments are run by the server.
		"/*!40014 SET FOREIGN_KEY_CHECKS=0 */":         "set",
		"/*!40000 ALTER TABLE `t` DISABLE KEYS */":     "alter",
		"/*!50001 CREATE VIEW v AS SELECT 1 FROM t */": "create",
	}

	for input, expected := range cases {
		assert.Equal(t, expected, firstKeyword(input), "input: %q", input)
	}
}

func TestReturnsResultSet(t *testing.T) {
	for _, keyword := range []string{"select", "show", "describe", "desc", "explain", "with", "values", "table", "call"} {
		assert.True(t, returnsResultSet(keyword), keyword)
	}

	for _, keyword := range []string{"insert", "update", "delete", "replace", "create", "drop", "alter", "truncate", "set", "grant"} {
		assert.False(t, returnsResultSet(keyword), keyword)
	}
}

func TestHasReturningClause(t *testing.T) {
	positive := []string{
		"INSERT INTO t (a) VALUES (1) RETURNING id",
		"insert into t values (1) returning *",
		"DELETE FROM t WHERE id = 1 RETURNING id, name",
		"REPLACE INTO t VALUES (1)\nRETURNING id",
		"INSERT INTO t VALUES (1--1) RETURNING id",          // -- without whitespace is subtraction, not a comment
		"INSERT INTO t VALUES (1) /* note */RETURNING id",   // a closed comment separates tokens
		"DELETE FROM t WHERE id = 1 RETURNING/* note */ id", // also after the keyword
	}

	for _, stmt := range positive {
		assert.True(t, hasReturningClause(firstKeyword(stmt), stmt), stmt)
	}

	negative := []string{
		"INSERT INTO t (note) VALUES ('use RETURNING here')", // inside string literal
		`INSERT INTO t (note) VALUES ("RETURNING too")`,      // inside double quotes
		"INSERT INTO t (a) VALUES (1) -- RETURNING id",       // inside line comment
		"INSERT INTO t (a) VALUES (1) # RETURNING id",        // inside hash comment
		"INSERT INTO t /* RETURNING */ VALUES (1)",           // inside block comment
		"INSERT INTO `returning` VALUES (1)",                 // quoted identifier
		"INSERT INTO t SET a = 1 - 1",                        // lone dash is not a comment
		"UPDATE t SET a = 1",                                 // keyword without clause
		"SELECT returning FROM t",                            // not a DML keyword
		"INSERT INTO t VALUES (returning_col)",               // no boundary after word
		"INSERT INTO t VALUES (1)",
	}

	for _, stmt := range negative {
		assert.False(t, hasReturningClause(firstKeyword(stmt), stmt), stmt)
	}
}

func TestSummarizeStatement(t *testing.T) {
	assert.Equal(t, "SELECT 1", summarizeStatement("SELECT\n\t1"))

	long := "SELECT " + strings.Repeat("a", 100)
	summary := summarizeStatement(long)
	assert.True(t, strings.HasSuffix(summary, "…"))
	assert.Less(t, len([]rune(summary)), 65)
}

func TestRenderTable(t *testing.T) {
	var out strings.Builder

	err := renderTable(
		[]string{"id", "name"},
		[][]any{
			{"1", "Standard rate"},
			{"2", nil},
		},
		time.Millisecond,
		&out,
	)
	require.NoError(t, err)

	expected := "+----+---------------+\n" +
		"| id | name          |\n" +
		"+----+---------------+\n" +
		"| 1  | Standard rate |\n" +
		"| 2  | NULL          |\n" +
		"+----+---------------+\n" +
		"2 rows in set (0.001 sec)\n"

	assert.Equal(t, expected, out.String())
}

func TestRenderTableEmptySet(t *testing.T) {
	var out strings.Builder

	require.NoError(t, renderTable([]string{"id"}, nil, time.Millisecond, &out))
	assert.Equal(t, "Empty set (0.001 sec)\n", out.String())
}

func TestRenderTSV(t *testing.T) {
	var out strings.Builder

	err := renderTSV(
		[]string{"id", "name"},
		[][]any{
			{"1", "with\ttab and\nnewline"},
			{"2", nil},
		},
		&out,
	)
	require.NoError(t, err)

	expected := "id\tname\n" +
		"1\twith\\ttab and\\nnewline\n" +
		"2\tNULL\n"

	assert.Equal(t, expected, out.String())
}

func TestRenderJSON(t *testing.T) {
	var out strings.Builder

	err := renderJSON(
		[]string{"id", "rate", "name"},
		[]bool{true, true, false},
		[][]any{
			{"1", "19.00", "Standard"},
			{"2", nil, nil},
		},
		&out,
	)
	require.NoError(t, err)

	assert.JSONEq(t, `[{"id":1,"rate":19.00,"name":"Standard"},{"id":2,"rate":null,"name":null}]`, out.String())
}

func TestIsExitCommand(t *testing.T) {
	for _, line := range []string{"exit", "quit", "EXIT", "exit;", `\q`, "  quit ; "} {
		assert.True(t, isExitCommand(line), line)
	}

	for _, line := range []string{"exits", "select 1", ""} {
		assert.False(t, isExitCommand(line), line)
	}
}
