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

			expectSingleTableDump(mock, "media", []dumpTestColumn{
				{name: "id", columnType: "int"},
				{name: "private", columnType: "tinyint(1)"},
			}, tc.mediaQuery, 7, 0)

			opts := tc.opts
			opts.Output = filepath.Join(t.TempDir(), "dump.sql")
			opts.SkipLockTables = true
			_, err = dumpDatabase(t.Context(), db, nil, opts)
			require.NoError(t, err)

			data, err := os.ReadFile(opts.Output)
			require.NoError(t, err)
			require.Contains(t, string(data), "INSERT INTO `media`")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
