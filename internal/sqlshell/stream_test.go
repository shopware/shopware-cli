package sqlshell

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExecer struct {
	statements []string
	failOn     string
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

func (f *fakeExecer) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	if f.failOn != "" && strings.Contains(query, f.failOn) {
		return nil, errors.New("boom")
	}

	f.statements = append(f.statements, query)

	return fakeResult{}, nil
}

// slowReader yields the input in tiny chunks so statements span reads.
type slowReader struct {
	data string
	pos  int
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}

	n := copy(p[:min(len(p), 3)], s.data[s.pos:])
	s.pos += n

	return n, nil
}

func TestExecuteStream(t *testing.T) {
	script := "SET NAMES utf8mb4;\n" +
		"-- structure of table t;\n" +
		"CREATE TABLE t (name VARCHAR(20));\n" +
		"/*!40014 SET FOREIGN_KEY_CHECKS=0 */;\n" +
		"INSERT INTO t VALUES ('a;b'), ('c');\n" +
		"/* plain comment */;\n" +
		"UPDATE t SET name = 'x' WHERE name = 'c'"

	db := &fakeExecer{}

	var counts []int
	count, err := ExecuteStream(t.Context(), db, &slowReader{data: script}, func(n int) {
		counts = append(counts, n)
	})
	require.NoError(t, err)

	assert.Equal(t, 5, count)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, counts)
	assert.Equal(t, []string{
		"SET NAMES utf8mb4",
		"-- structure of table t;\nCREATE TABLE t (name VARCHAR(20))",
		"/*!40014 SET FOREIGN_KEY_CHECKS=0 */",
		"INSERT INTO t VALUES ('a;b'), ('c')",
		"UPDATE t SET name = 'x' WHERE name = 'c'",
	}, db.statements)
}

func TestExecuteStreamTriggerDump(t *testing.T) {
	// Mirrors the trigger section emitted by internal/mysqldump, fed in
	// 3-byte chunks so the delimiter state must survive chunk boundaries.
	script := "DROP TRIGGER IF EXISTS `order_update`;\n" +
		"DELIMITER //\n" +
		"CREATE TRIGGER order_update BEFORE UPDATE ON `order` FOR EACH ROW BEGIN\n" +
		"  SET NEW.updated_at = NOW();\n" +
		"  SET @counter = @counter + 1;\n" +
		"END//\n" +
		"DELIMITER ;\n" +
		"INSERT INTO t VALUES (1);\n"

	db := &fakeExecer{}

	count, err := ExecuteStream(t.Context(), db, &slowReader{data: script}, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, count)
	assert.Equal(t, []string{
		"DROP TRIGGER IF EXISTS `order_update`",
		"CREATE TRIGGER order_update BEFORE UPDATE ON `order` FOR EACH ROW BEGIN\n" +
			"  SET NEW.updated_at = NOW();\n" +
			"  SET @counter = @counter + 1;\n" +
			"END",
		"INSERT INTO t VALUES (1)",
	}, db.statements)
}

func TestExecuteStreamTrailingDelimiterDirective(t *testing.T) {
	db := &fakeExecer{}

	// No trailing newline after the final directive: it must not be sent to
	// the server as SQL.
	count, err := ExecuteStream(t.Context(), db, strings.NewReader("SELECT 1;\nDELIMITER ;"), nil)
	require.NoError(t, err)

	assert.Equal(t, 1, count)
	assert.Equal(t, []string{"SELECT 1"}, db.statements)
}

func TestExecuteStreamStopsOnError(t *testing.T) {
	db := &fakeExecer{failOn: "two"}

	count, err := ExecuteStream(t.Context(), db, strings.NewReader("SELECT one; SELECT two; SELECT three;"), nil)
	require.Error(t, err)

	assert.Equal(t, 1, count)
	assert.Contains(t, err.Error(), "SELECT two")
	assert.Equal(t, []string{"SELECT one"}, db.statements)
}

func TestExecuteStreamEmptyInput(t *testing.T) {
	db := &fakeExecer{}

	count, err := ExecuteStream(t.Context(), db, strings.NewReader("\n-- nothing to do\n"), nil)
	require.NoError(t, err)

	assert.Equal(t, 0, count)
	assert.Empty(t, db.statements)
}
