package sqlshell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		statements []string
		rest       string
	}{
		{
			name:       "single statement",
			input:      "SELECT 1;",
			statements: []string{"SELECT 1"},
		},
		{
			name:       "multiple statements",
			input:      "SELECT 1; SELECT 2;\nSELECT 3;",
			statements: []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		},
		{
			name:  "incomplete statement",
			input: "SELECT * FROM product",
			rest:  "SELECT * FROM product",
		},
		{
			name:       "complete and incomplete",
			input:      "SELECT 1; SELECT 2",
			statements: []string{"SELECT 1"},
			rest:       "SELECT 2",
		},
		{
			name:       "semicolon in single quotes",
			input:      "SELECT 'a;b';",
			statements: []string{"SELECT 'a;b'"},
		},
		{
			name:       "semicolon in double quotes",
			input:      `SELECT "a;b";`,
			statements: []string{`SELECT "a;b"`},
		},
		{
			name:       "semicolon in backticks",
			input:      "SELECT `a;b` FROM t;",
			statements: []string{"SELECT `a;b` FROM t"},
		},
		{
			name:       "escaped quote in string",
			input:      `SELECT 'it\'s;ok';`,
			statements: []string{`SELECT 'it\'s;ok'`},
		},
		{
			name:       "doubled quote in string",
			input:      "SELECT 'it''s;ok';",
			statements: []string{"SELECT 'it''s;ok'"},
		},
		{
			name:       "semicolon in line comment",
			input:      "SELECT 1 -- comment;\n;",
			statements: []string{"SELECT 1 -- comment;"},
		},
		{
			name:       "semicolon in hash comment",
			input:      "SELECT 1 # comment;\n;",
			statements: []string{"SELECT 1 # comment;"},
		},
		{
			name:       "semicolon in block comment",
			input:      "SELECT /* ; */ 1;",
			statements: []string{"SELECT /* ; */ 1"},
		},
		{
			name:       "double dash without space is no comment",
			input:      "SELECT 1--2;",
			statements: []string{"SELECT 1--2"},
		},
		{
			name:       "empty statements are dropped",
			input:      ";;  ;SELECT 1;",
			statements: []string{"SELECT 1"},
		},
		{
			name:  "unterminated string keeps rest",
			input: "SELECT 'abc",
			rest:  "SELECT 'abc",
		},
		{
			name: "delimiter directive for trigger",
			input: "DROP TRIGGER IF EXISTS `t`;\n" +
				"DELIMITER //\n" +
				"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; SET @b = 2; END//\n" +
				"DELIMITER ;\n" +
				"SELECT 1;",
			statements: []string{
				"DROP TRIGGER IF EXISTS `t`",
				"CREATE TRIGGER t BEFORE INSERT ON x FOR EACH ROW BEGIN SET @a = 1; SET @b = 2; END",
				"SELECT 1",
			},
		},
		{
			name:       "delimiter directive is case-insensitive",
			input:      "delimiter $$\nSELECT 1$$SELECT 2$$",
			statements: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:       "delimiter word inside statement is content",
			input:      "SELECT 'DELIMITER //';",
			statements: []string{"SELECT 'DELIMITER //'"},
		},
		{
			name:  "incomplete delimiter directive stays in rest",
			input: "SELECT 1;\nDELIMITER /",
			statements: []string{
				"SELECT 1",
			},
			rest: "DELIMITER /",
		},
		{
			name:       "delimiter directive without token keeps delimiter",
			input:      "DELIMITER \nSELECT 1;",
			statements: []string{"SELECT 1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statements, rest := SplitStatements(tc.input)
			assert.Equal(t, tc.statements, statements)
			assert.Equal(t, tc.rest, rest)
		})
	}
}

func TestSplitStatementsCarriesDelimiterAcrossCalls(t *testing.T) {
	statements, rest, delimiter := SplitStatementsWithDelimiter("DELIMITER //\nCREATE TRIGGER t BEGIN SET @a = 1; ", DefaultDelimiter)
	assert.Empty(t, statements)
	assert.Equal(t, "CREATE TRIGGER t BEGIN SET @a = 1; ", rest)
	assert.Equal(t, "//", delimiter)

	statements, rest, delimiter = SplitStatementsWithDelimiter(rest+"END//\nDELIMITER ;\nSELECT 1;", delimiter)
	assert.Equal(t, []string{"CREATE TRIGGER t BEGIN SET @a = 1; END", "SELECT 1"}, statements)
	assert.Empty(t, rest)
	assert.Equal(t, ";", delimiter)
}
