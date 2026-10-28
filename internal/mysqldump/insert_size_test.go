package mysqldump

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDumpTableDataInsertSize(t *testing.T) {
	header := "INSERT INTO `images` (`content`) VALUES\n"
	twoRowsSize := len(header) + len("( 'a' ),\n( 'b' );\n")
	large := strings.Repeat("a", DefaultInsertIntoMaxBytes/2)
	for _, tc := range []struct {
		name     string
		values   []string
		rows     []string
		groups   []int
		maxBytes int
		rowLimit int
		binary   bool
	}{
		{name: "exact boundary", values: []string{"a", "b", "c"}, rows: []string{"( 'a' )", "( 'b' )", "( 'c' )"}, maxBytes: twoRowsSize, groups: []int{2, 1}},
		{name: "one byte over boundary", values: []string{"a", "b"}, rows: []string{"( 'a' )", "( 'b' )"}, maxBytes: twoRowsSize - 1, groups: []int{1, 1}},
		{name: "row limit still applies", values: []string{"a", "b", "c"}, rows: []string{"( 'a' )", "( 'b' )", "( 'c' )"}, maxBytes: 1024, rowLimit: 2, groups: []int{2, 1}},
		{name: "oversized row stays intact", values: []string{"a", strings.Repeat("b", 100), "c"}, rows: []string{"( 'a' )", "( '" + strings.Repeat("b", 100) + "' )", "( 'c' )"}, maxBytes: twoRowsSize, groups: []int{1, 1, 1}},
		{name: "escaped bytes count", values: []string{"'", "'"}, rows: []string{`( '\'' )`, `( '\'' )`}, maxBytes: twoRowsSize, groups: []int{1, 1}},
		{name: "utf8 bytes count", values: []string{"ä", "ä"}, rows: []string{"( 'ä' )", "( 'ä' )"}, maxBytes: twoRowsSize, groups: []int{1, 1}},
		{name: "hex bytes count", values: []string{"abcd", "abcd"}, rows: []string{"( 0x" + hex.EncodeToString([]byte("abcd")) + " )", "( 0x" + hex.EncodeToString([]byte("abcd")) + " )"}, binary: true, maxBytes: twoRowsSize, groups: []int{1, 1}},
		{name: "default size limit", values: []string{large, large}, rows: []string{"( '" + large + "' )", "( '" + large + "' )"}, groups: []int{1, 1}},
		{name: "empty table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := getDB(t)
			t.Cleanup(func() { _ = db.Close() })
			dumper := NewMySQLDumper(db)
			dumper.InsertIntoMaxBytes = tc.maxBytes
			dumper.InsertIntoLimit = tc.rowLimit
			if tc.binary {
				dumper.mapBins["images"] = []string{"content"}
			}
			for range 2 {
				mock.ExpectQuery("SELECT \\* FROM `images` LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"content"}))
			}
			rows := sqlmock.NewRows([]string{"content"})
			for _, value := range tc.values {
				rows.AddRow(value)
			}
			mock.ExpectQuery("SELECT `content` FROM `images`").WillReturnRows(rows)
			var output bytes.Buffer
			require.NoError(t, dumper.dumpTableData(t.Context(), &output, "images"))
			require.NoError(t, mock.ExpectationsWereMet())

			var expected strings.Builder
			offset := 0
			for _, count := range tc.groups {
				expected.WriteString(header)
				expected.WriteString(strings.Join(tc.rows[offset:offset+count], ",\n"))
				expected.WriteString(";\n")
				offset += count
			}
			assert.Equal(t, len(tc.rows), offset)
			assert.Equal(t, expected.String(), output.String())
		})
	}
}

func TestDumpTableDataRowIterationError(t *testing.T) {
	db, mock := getDB(t)
	t.Cleanup(func() { _ = db.Close() })
	dumper := NewMySQLDumper(db)
	for range 2 {
		mock.ExpectQuery("SELECT \\* FROM `images` LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"content"}))
	}
	wantErr := errors.New("connection lost")
	rows := sqlmock.NewRows([]string{"content"}).AddRow("first").AddRow("second").RowError(1, wantErr)
	mock.ExpectQuery("SELECT `content` FROM `images`").WillReturnRows(rows)
	var output bytes.Buffer
	assert.ErrorIs(t, dumper.dumpTableData(t.Context(), &output, "images"), wantErr)
	require.NoError(t, mock.ExpectationsWereMet())
}
