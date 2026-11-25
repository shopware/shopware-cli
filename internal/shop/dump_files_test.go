package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDumpDatabaseSkipFileRowsFiltersMedia(t *testing.T) {
	cases := []struct {
		name       string
		opts       DumpDatabaseOptions
		mediaQuery string
	}{
		{name: "all files", mediaQuery: "^SELECT .* FROM `media`$"},
		{name: "without public files", opts: DumpDatabaseOptions{SkipPublicFileRows: true}, mediaQuery: "^SELECT .* FROM `media` WHERE .*`private` = 0.*IS NOT TRUE"},
		{name: "without private files", opts: DumpDatabaseOptions{SkipPrivateFileRows: true}, mediaQuery: "^SELECT .* FROM `media` WHERE .*`private` = 1.*IS NOT TRUE"},
		{name: "without any files", opts: DumpDatabaseOptions{SkipPublicFileRows: true, SkipPrivateFileRows: true}, mediaQuery: "^SELECT .* FROM `media` WHERE .*TRUE.*IS NOT TRUE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
				sqlmock.NewRows([]string{"table", "type"}).AddRow("media", "BASE TABLE"))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
					AddRow("media", "InnoDB", "utf8mb4_unicode_ci", "", "Dynamic", nil))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT", "EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION"}).
					AddRow("media", "id", "int", nil, "NO", nil, "", nil, "", nil).
					AddRow("media", "private", "tinyint(1)", nil, "NO", "0", "", nil, "", nil))
			mock.ExpectQuery("SELECT COUNT\\(\\*\\).*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").WillReturnRows(
				sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX"}))
			mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
			mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
			mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION"}))
			mock.ExpectQuery("SELECT COUNT.*FROM INFORMATION_SCHEMA.COLUMNS.*TABLE_NAME = 'CHECK_CONSTRAINTS'.*COLUMN_NAME = 'TABLE_NAME'").
				WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE"}))
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `media`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			for range 2 {
				mock.ExpectQuery("SELECT \\* FROM `media` LIMIT 1").WillReturnRows(
					sqlmock.NewRows([]string{"id", "private"}))
			}
			mock.ExpectQuery(tc.mediaQuery).WillReturnRows(
				sqlmock.NewRows([]string{"id", "private"}).AddRow(7, 0))
			mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(sqlmock.NewRows([]string{"table", "type"}))
			mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(sqlmock.NewRows([]string{"trigger", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}))

			opts := tc.opts
			opts.Output = filepath.Join(t.TempDir(), "dump.sql")
			opts.SkipLockTables = true
			require.NoError(t, dumpDatabase(t.Context(), db, nil, opts))

			data, err := os.ReadFile(opts.Output)
			require.NoError(t, err)
			require.Contains(t, string(data), "INSERT INTO `media`")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
