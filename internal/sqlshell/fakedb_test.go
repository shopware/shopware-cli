package sqlshell

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fake driver serves canned result sets keyed by the exact query text
// and lets the rendering pipeline run on real *sql.Rows without a server.
// Queries containing "boom" fail, exec statements containing "one_row"
// report one affected row, all others three.

type fakeTable struct {
	cols  []string
	types []string
	rows  [][]driver.Value
}

var fakeQueries sync.Map // query string -> fakeTable

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return &fakeDBConn{}, nil }

type fakeDBConn struct{}

func (*fakeDBConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (*fakeDBConn) Close() error { return nil }

func (*fakeDBConn) Begin() (driver.Tx, error) { return nil, errors.New("tx not supported") }

func (*fakeDBConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "boom") {
		return nil, errors.New("query exploded")
	}

	// Simulates a long-running query: blocks until the context is cancelled.
	if strings.Contains(query, "block") {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	table, ok := fakeQueries.Load(query)
	if !ok {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}

	return &fakeDriverRows{table: table.(fakeTable)}, nil
}

func (*fakeDBConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "boom") {
		return nil, errors.New("exec exploded")
	}

	if strings.Contains(query, "one_row") {
		return driver.RowsAffected(1), nil
	}

	return driver.RowsAffected(3), nil
}

type fakeDriverRows struct {
	table fakeTable
	pos   int
}

func (r *fakeDriverRows) Columns() []string { return r.table.cols }

func (r *fakeDriverRows) Close() error { return nil }

func (r *fakeDriverRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.table.rows) {
		return io.EOF
	}

	copy(dest, r.table.rows[r.pos])
	r.pos++

	return nil
}

func (r *fakeDriverRows) ColumnTypeDatabaseTypeName(index int) string {
	return r.table.types[index]
}

var registerFakeDriver = sync.OnceFunc(func() {
	sql.Register("sqlshell-fake", fakeDriver{})
})

func openFakeDB(t *testing.T) *sql.DB {
	t.Helper()
	registerFakeDriver()

	db, err := sql.Open("sqlshell-fake", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
