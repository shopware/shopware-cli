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

			appQuery := "^SELECT .* FROM `app`$"
			if resetIdentity {
				appQuery = "^SELECT .* FROM `app` WHERE .*app_secret.*IS NOT NULL.*IS NOT TRUE"
			}
			expectSingleTableDump(mock, "app", []dumpTestColumn{
				{name: "id", columnType: "int"},
				{name: "app_secret", columnType: "varchar(255)", nullable: true},
				{name: "name", columnType: "varchar(255)"},
			}, appQuery, 7, nil, "local-app")

			output := filepath.Join(t.TempDir(), "dump.sql")
			_, err = dumpDatabase(t.Context(), db, nil, DumpDatabaseOptions{
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
