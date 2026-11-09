package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDumpDatabaseResetIdentityFiltersExportedAppRows(t *testing.T) {
	for _, resetIdentity := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[resetIdentity], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(
				sqlmock.NewRows([]string{"table", "type"}).AddRow("app", "BASE TABLE"))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLES.*TABLE_TYPE = 'BASE TABLE'").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "ENGINE", "TABLE_COLLATION", "TABLE_COMMENT", "ROW_FORMAT", "AUTO_INCREMENT"}).
					AddRow("app", "InnoDB", "utf8mb4_unicode_ci", "", "Dynamic", nil))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.COLUMNS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "COLUMN_NAME", "COLUMN_TYPE", "CHARACTER_SET_NAME", "IS_NULLABLE", "COLUMN_DEFAULT", "EXTRA", "COLLATION_NAME", "COLUMN_COMMENT", "GENERATION_EXPRESSION"}).
					AddRow("app", "id", "int", nil, "NO", nil, "", nil, "", nil).
					AddRow("app", "app_secret", "varchar(255)", "utf8mb4", "YES", nil, "", "utf8mb4_unicode_ci", "", nil).
					AddRow("app", "name", "varchar(255)", "utf8mb4", "NO", nil, "", "utf8mb4_unicode_ci", "", nil))
			mock.ExpectQuery("SELECT COUNT\\(\\*\\).*TABLE_NAME = 'STATISTICS'.*COLUMN_NAME = 'EXPRESSION'").WillReturnRows(
				sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.STATISTICS.*WHERE TABLE_SCHEMA = DATABASE()").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "INDEX_NAME", "COLUMN_NAME", "EXPRESSION", "NON_UNIQUE", "INDEX_TYPE", "SUB_PART", "COLLATION", "INDEX_COMMENT", "SEQ_IN_INDEX"}))
			mock.ExpectQuery("SELECT COUNT.*KEY_COLUMN_USAGE.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
			mock.ExpectQuery("SELECT COUNT.*REFERENTIAL_CONSTRAINTS.*").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
			mock.ExpectQuery("SELECT DISTINCT.*FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE.*").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "COLUMN_NAME", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME", "UPDATE_RULE", "DELETE_RULE", "ORDINAL_POSITION"}))
			mock.ExpectQuery("SELECT.*FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS.*CHECK_CONSTRAINTS.*").WillReturnRows(
				sqlmock.NewRows([]string{"TABLE_NAME", "CONSTRAINT_NAME", "CHECK_CLAUSE"}))
			mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `app`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			for range 2 {
				mock.ExpectQuery("SELECT \\* FROM `app` LIMIT 1").WillReturnRows(
					sqlmock.NewRows([]string{"id", "app_secret", "name"}))
			}
			appQuery := "^SELECT .* FROM `app`$"
			if resetIdentity {
				appQuery = "^SELECT .* FROM `app` WHERE .*app_secret.*IS NOT NULL.*IS NOT TRUE"
			}
			mock.ExpectQuery(appQuery).WillReturnRows(
				sqlmock.NewRows([]string{"id", "app_secret", "name"}).AddRow(7, nil, "local-app"))
			mock.ExpectQuery("SHOW FULL TABLES").WillReturnRows(sqlmock.NewRows([]string{"table", "type"}))
			mock.ExpectQuery("SHOW TRIGGERS").WillReturnRows(sqlmock.NewRows([]string{"trigger", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}))

			output := filepath.Join(t.TempDir(), "dump.sql")
			err = dumpDatabase(t.Context(), db, nil, DumpDatabaseOptions{
				Output: output, SkipLockTables: true, ResetIdentity: resetIdentity,
			})
			require.NoError(t, err)

			data, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Contains(t, string(data), "'local-app'")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
